package store_test

import (
	"testing"
	"time"

	"queueup/internal/store"
)

// The three emails' windows, as the relay sets them. Kept here as numbers so
// a change to the schedule has to be made deliberately in both places.
var windows = []struct {
	stage       store.WinbackStage
	from, until time.Duration
}{
	{1, 2 * time.Hour, 24 * time.Hour},
	{2, 24 * time.Hour, 68 * time.Hour},
	{3, 68 * time.Hour, 72 * time.Hour},
}

// signedUp makes an account that is `age` old and arrived on a promo code.
func signedUp(t *testing.T, s *store.Store, email string, age time.Duration, now time.Time) store.Account {
	t.Helper()
	a, err := s.Register(email, "a good password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`,
		now.Add(-age).UnixMilli(), a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RememberSourceCode(a.ID, "TIKTOK"); err != nil {
		t.Fatal(err)
	}
	return a
}

// due is which of the three emails, if any, an account would get right now.
func due(t *testing.T, s *store.Store, id string, now time.Time) []store.WinbackStage {
	t.Helper()
	var out []store.WinbackStage
	for _, w := range windows {
		list, err := s.AccountsDueWinback(now, w.stage, w.from, w.until)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range list {
			if a.ID == id {
				out = append(out, w.stage)
			}
		}
	}
	return out
}

// Each email belongs to a stretch of time, and somebody arriving late gets
// only the one that is current — never every email they missed, one after
// another. And nothing at all after the offer has ended.
func TestEachEmailHasItsOwnWindow(t *testing.T) {
	now := time.Now()
	cases := []struct {
		age  time.Duration
		want []store.WinbackStage
	}{
		{1 * time.Hour, nil}, // too soon: they may still be on the page
		{3 * time.Hour, []store.WinbackStage{1}},
		{23 * time.Hour, []store.WinbackStage{1}},
		// The case the windows exist for: signed up a day and a bit before
		// this went live. Email two, and only email two.
		{30 * time.Hour, []store.WinbackStage{2}},
		{67 * time.Hour, []store.WinbackStage{2}},
		{70 * time.Hour, []store.WinbackStage{3}},
		// Past the offer. "Ends in four hours" would be untrue.
		{73 * time.Hour, nil},
		{10 * 24 * time.Hour, nil},
	}
	for _, c := range cases {
		s := newStore(t)
		a := signedUp(t, s, "late@example.com", c.age, now)
		got := due(t, s, a.ID, now)
		if len(got) != len(c.want) || (len(got) == 1 && got[0] != c.want[0]) {
			t.Errorf("signed up %v ago: due %v, want %v", c.age, got, c.want)
		}
	}
}

// Once an email is claimed, that one and every earlier one are done.
func TestAnEmailIsSentOnceAndNeverRewound(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	a := signedUp(t, s, "once@example.com", 30*time.Hour, now)

	ok, err := s.MarkWinbackSent(a.ID, 2)
	if err != nil || !ok {
		t.Fatalf("first claim = %v, %v", ok, err)
	}
	// A second relay, or a second tick, racing for the same email.
	if ok, _ := s.MarkWinbackSent(a.ID, 2); ok {
		t.Error("the same email was claimed twice")
	}
	// The skipped first email must never turn up afterwards.
	if ok, _ := s.MarkWinbackSent(a.ID, 1); ok {
		t.Error("an earlier email was sent after a later one")
	}
	if got := due(t, s, a.ID, now); len(got) != 0 {
		t.Errorf("still due %v after email two went", got)
	}
	// Later, the third one is still theirs.
	if got := due(t, s, a.ID, now.Add(40*time.Hour)); len(got) != 1 || got[0] != 3 {
		t.Errorf("at 70 hours, due %v, want [3]", got)
	}
}

// Every reason not to send marketing to somebody, each one on its own.
func TestNobodyWhoShouldNotGetTheseDoes(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		setup func(*store.Store, store.Account)
	}{
		{"paid", func(s *store.Store, a store.Account) {
			_ = s.SetSubscription(a.ID, "active", "sub_1")
		}},
		{"on free access", func(s *store.Store, a store.Account) {
			_ = s.SetSubscription(a.ID, store.StatusComped, "")
		}},
		{"said stop", func(s *store.Store, a store.Account) {
			tok, _ := s.UnsubscribeToken(a.ID)
			_ = s.OptOutOfEmail(a.ID, tok)
		}},
		{"asked to be deleted", func(s *store.Store, a store.Account) {
			_ = s.ExecForTests(`UPDATE accounts SET erase_after = ? WHERE id = ?`,
				now.Add(7*24*time.Hour).UnixMilli(), a.ID)
		}},
		{"arrived with no code, so has no offer", func(s *store.Store, a store.Account) {
			_ = s.ExecForTests(`UPDATE accounts SET source_code = '' WHERE id = ?`, a.ID)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newStore(t)
			a := signedUp(t, s, "nope@example.com", 3*time.Hour, now)
			if got := due(t, s, a.ID, now); len(got) != 1 {
				t.Fatalf("before %s: due %v, want one email", c.name, got)
			}
			c.setup(s, a)
			if got := due(t, s, a.ID, now); len(got) != 0 {
				t.Errorf("%s, and still due %v", c.name, got)
			}
		})
	}
}

// Paying halfway through ends the sequence. There is nothing to cancel,
// because nothing was scheduled: the next email simply does not find them.
func TestPayingEndsTheSequenceAtOnce(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	a := signedUp(t, s, "payer@example.com", 3*time.Hour, now)
	if ok, _ := s.MarkWinbackSent(a.ID, 1); !ok {
		t.Fatal("could not send the first email")
	}
	if err := s.SetSubscription(a.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}
	for _, later := range []time.Duration{25 * time.Hour, 69 * time.Hour} {
		if got := due(t, s, a.ID, now.Add(later)); len(got) != 0 {
			t.Errorf("paid, and %v later still due %v", later, got)
		}
	}
}

// The unsubscribe link: one token per account, stable, and good only for
// that account. Saying stop twice is fine; nothing ever turns it back on.
func TestUnsubscribing(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	a := signedUp(t, s, "leave@example.com", 3*time.Hour, now)
	b := signedUp(t, s, "other@example.com", 3*time.Hour, now)

	tok, err := s.UnsubscribeToken(a.ID)
	if err != nil || len(tok) < 20 {
		t.Fatalf("token = %q, %v", tok, err)
	}
	// The same token every time, so every email they hold still works.
	if again, _ := s.UnsubscribeToken(a.ID); again != tok {
		t.Error("the token changed between emails, breaking the older links")
	}
	otherTok, _ := s.UnsubscribeToken(b.ID)
	if otherTok == tok {
		t.Fatal("two accounts share an unsubscribe token")
	}

	// Somebody else's token, or a made-up one, does nothing — and says
	// nothing about why, so the page cannot be used to probe for accounts.
	for _, bad := range []struct{ id, tok string }{
		{a.ID, otherTok},
		{a.ID, "made-up"},
		{"acct_nobody", tok},
		{a.ID, ""},
	} {
		if err := s.OptOutOfEmail(bad.id, bad.tok); err != store.ErrBadUnsubscribe {
			t.Errorf("OptOutOfEmail(%q, %q) = %v, want ErrBadUnsubscribe", bad.id, bad.tok, err)
		}
	}
	if out, _ := s.OptedOutOfEmail(a.ID); out {
		t.Fatal("a bad token opted somebody out")
	}

	if err := s.OptOutOfEmail(a.ID, tok); err != nil {
		t.Fatal(err)
	}
	if err := s.OptOutOfEmail(a.ID, tok); err != nil {
		t.Errorf("pressing it a second time failed: %v", err)
	}
	if out, _ := s.OptedOutOfEmail(a.ID); !out {
		t.Error("not opted out")
	}
	if out, _ := s.OptedOutOfEmail(b.ID); out {
		t.Error("opting one account out opted out another")
	}
}
