package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"queueup/internal/servers"
	"queueup/internal/store"
	"queueup/internal/stripe"
)

type winbackRig struct {
	srv  *Server
	st   *store.Store
	ts   *httptest.Server
	mail *fakeResend
}

func newWinbackRig(t *testing.T, demo, testimonial string) *winbackRig {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake, sender := newFakeResend(t)
	fs := newFakeStripe(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{
		Store: st, Log: quiet, Servers: servers.NewStub(), Mail: sender,
		WebURL:              "https://queueuprust.com",
		BillingEnabled:      true,
		Stripe:              &stripe.Client{SecretKey: "sk_test_fake", BaseURL: fs.ts.URL},
		StripePriceID:       testPriceID,
		StripeIntroCouponID: testCouponID,
		DemoURL:             demo,
		Testimonial:         testimonial,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &winbackRig{srv: srv, st: st, ts: ts, mail: fake}
}

// signup makes an account the way the landing page does — an email and no
// password — that arrived on `code` and is `age` old at `now`.
func (r *winbackRig) signup(t *testing.T, email, code string, age time.Duration, now time.Time) store.Account {
	t.Helper()
	a, err := r.st.StartAccount(email)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.st.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`,
		now.Add(-age).UnixMilli(), a.ID); err != nil {
		t.Fatal(err)
	}
	if code != "" {
		if err := r.st.RememberSourceCode(a.ID, code); err != nil {
			t.Fatal(err)
		}
	}
	a.CreatedAt = time.UnixMilli(now.Add(-age).UnixMilli()).UTC()
	return a
}

// sent is every email so far. sendWinback runs in the foreground, so there
// is nothing to wait for.
func (r *winbackRig) sent() []map[string]any {
	r.mail.mu.Lock()
	defer r.mail.mu.Unlock()
	return append([]map[string]any(nil), r.mail.sent...)
}

func text(m map[string]any) string    { s, _ := m["text"].(string); return s }
func subject(m map[string]any) string { s, _ := m["subject"].(string); return s }

// unsubscribeFrom pulls the unsubscribe link out of an email body.
func unsubscribeFrom(t *testing.T, body string) url.Values {
	t.Helper()
	i := strings.Index(body, "https://queueuprust.com/unsubscribe?")
	if i < 0 {
		t.Fatalf("no unsubscribe link in:\n%s", body)
	}
	line := strings.Fields(body[i:])[0]
	u, err := url.Parse(line)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

// The whole sequence, as one person living through it. Each email at its
// time, saying what it should, carrying a way out, and then silence.
func TestTheSequenceSendsTheRightEmailAtTheRightTime(t *testing.T) {
	r := newWinbackRig(t, "", "")
	start := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) // 10:00 in London (BST)
	a := r.signup(t, "tiktok@example.com", "TIKTOK", 0, start)

	// Too soon: they may still be on the page.
	r.srv.sendWinback(context.Background(), start.Add(time.Hour))
	if n := len(r.sent()); n != 0 {
		t.Fatalf("%d emails an hour after signup", n)
	}

	// Email one.
	r.srv.sendWinback(context.Background(), start.Add(3*time.Hour))
	got := r.sent()
	if len(got) != 1 {
		t.Fatalf("after 3 hours: %d emails, want 1", len(got))
	}
	one := got[0]
	if s := subject(one); s != "Your £1.99 first month is saved" {
		t.Errorf("email one subject = %q", s)
	}
	body := text(one)
	for _, want := range []string{
		"£1.99",
		"TIKTOK",
		// The deadline as hours left: sent three hours after signup, so 69.
		"for the next 69 hours",
		// One tap back to their own paywall, with the code riding along.
		"https://queueuprust.com/continue?a=" + a.ID,
		"promo=TIKTOK",
		"£4.99 a month",
		"not affiliated with Facepunch",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("email one is missing %q:\n%s", want, body)
		}
	}
	// The unsubscribe link, in the body and in the header mail apps read.
	q := unsubscribeFrom(t, body)
	if q.Get("a") != a.ID || q.Get("t") == "" {
		t.Errorf("unsubscribe link carries %v", q)
	}
	headers, _ := one["headers"].(map[string]any)
	if lu, _ := headers["List-Unsubscribe"].(string); !strings.Contains(lu, "/unsubscribe?") {
		t.Errorf("no List-Unsubscribe header: %v", one["headers"])
	}

	// Running again inside the same window sends nothing new.
	r.srv.sendWinback(context.Background(), start.Add(10*time.Hour))
	if n := len(r.sent()); n != 1 {
		t.Fatalf("email one went %d times", n)
	}

	// Email two.
	r.srv.sendWinback(context.Background(), start.Add(25*time.Hour))
	got = r.sent()
	if len(got) != 2 {
		t.Fatalf("after 25 hours: %d emails, want 2", len(got))
	}
	two := text(got[1])
	if s := subject(got[1]); !strings.Contains(s, "allowed") {
		t.Errorf("email two subject = %q", s)
	}
	for _, want := range []string{"cheat", "memory", "Steam", "cancel", "keep the days", "for the next 47 hours", "/unsubscribe?"} {
		if !strings.Contains(two, want) {
			t.Errorf("email two is missing %q:\n%s", want, two)
		}
	}
	// The decision: cancel anytime, never a refund promise.
	if strings.Contains(strings.ToLower(two), "refund") {
		t.Errorf("email two promises a refund:\n%s", two)
	}
	// It must not promise nobody gets banned: that is not ours to promise.
	if strings.Contains(strings.ToLower(two), "won't get banned") ||
		strings.Contains(strings.ToLower(two), "never be banned") {
		t.Errorf("email two guarantees no bans:\n%s", two)
	}
	// No demo clip and no testimonial set, so neither line appears at all.
	if strings.Contains(two, "Watch it") {
		t.Errorf("email two has a demo line with no demo:\n%s", two)
	}

	// Email three, four hours before the end.
	r.srv.sendWinback(context.Background(), start.Add(68*time.Hour+5*time.Minute))
	got = r.sent()
	if len(got) != 3 {
		t.Fatalf("after 68 hours: %d emails, want 3", len(got))
	}
	if s := subject(got[2]); s != "Your £1.99 month ends in 4 hours" {
		t.Errorf("email three subject = %q", s)
	}
	if b := text(got[2]); !strings.Contains(b, "ends in 4 hours") || !strings.Contains(b, "/unsubscribe?") {
		t.Errorf("email three:\n%s", b)
	}

	// Hours only: no date and no clock time anywhere in any of them.
	clock := regexp.MustCompile(`\d(am|pm)\b|\d:\d\d|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday|October|today|tomorrow`)
	for i, m := range got {
		if hit := clock.FindString(subject(m) + "\n" + text(m)); hit != "" {
			t.Errorf("email %d gives a date or time (%q), not hours left:\n%s", i+1, hit, text(m))
		}
	}

	// And then nothing, ever.
	for _, later := range []time.Duration{71 * time.Hour, 80 * time.Hour, 30 * 24 * time.Hour} {
		r.srv.sendWinback(context.Background(), start.Add(later))
	}
	if n := len(r.sent()); n != 3 {
		t.Errorf("%d emails in total, want exactly 3", n)
	}
}

// Paying halfway through stops the rest, straight away.
func TestPayingStopsTheRestOfTheSequence(t *testing.T) {
	r := newWinbackRig(t, "", "")
	start := time.Now().Add(-3 * time.Hour)
	a := r.signup(t, "payer@example.com", "TIKTOK", 0, start)

	r.srv.sendWinback(context.Background(), start.Add(3*time.Hour))
	if n := len(r.sent()); n != 1 {
		t.Fatalf("%d emails before paying, want 1", n)
	}
	if err := r.st.SetSubscription(a.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}
	for _, later := range []time.Duration{25 * time.Hour, 69 * time.Hour} {
		r.srv.sendWinback(context.Background(), start.Add(later))
	}
	if n := len(r.sent()); n != 1 {
		t.Errorf("paid, and still got %d emails", n)
	}
}

// Somebody who signed up a day and a half before this went live gets the
// email that is current for them, not every one they missed.
func TestALateArrivalGetsOneEmailNotThree(t *testing.T) {
	r := newWinbackRig(t, "", "")
	now := time.Now()
	r.signup(t, "late@example.com", "TIKTOK", 30*time.Hour, now)

	r.srv.sendWinback(context.Background(), now)
	r.srv.sendWinback(context.Background(), now.Add(5*time.Minute))
	got := r.sent()
	if len(got) != 1 {
		t.Fatalf("%d emails, want 1", len(got))
	}
	if !strings.Contains(subject(got[0]), "allowed") {
		t.Errorf("got %q, want email two", subject(got[0]))
	}
}

// A code that does not exist in Stripe gives no £1.99, so no email may say
// it does. And nobody without a code has an offer to be told about at all.
func TestNoEmailPromisesAPriceStripeWontCharge(t *testing.T) {
	r := newWinbackRig(t, "", "")
	now := time.Now()
	dead := r.signup(t, "typo@example.com", "TIKTOOK", 3*time.Hour, now)
	r.signup(t, "direct@example.com", "", 3*time.Hour, now)

	r.srv.sendWinback(context.Background(), now)
	if n := len(r.sent()); n != 0 {
		t.Fatalf("%d emails to people with no live offer", n)
	}

	// The dead code is not spent: if they type a working one later, the
	// sequence picks them up.
	if err := r.st.RememberSourceCode(dead.ID, "TIKTOK"); err != nil {
		t.Fatal(err)
	}
	r.srv.sendWinback(context.Background(), now)
	if n := len(r.sent()); n != 1 {
		t.Errorf("after a working code: %d emails, want 1", n)
	}
}

// The two optional lines in email two appear when set and not otherwise.
func TestTheDemoAndTestimonialLinesAppearOnlyWhenSet(t *testing.T) {
	r := newWinbackRig(t, "https://www.tiktok.com/@queueup/video/1", `"Got me in on wipe day from work." - Jake`)
	now := time.Now()
	r.signup(t, "two@example.com", "TIKTOK", 25*time.Hour, now)
	r.srv.sendWinback(context.Background(), now)
	got := r.sent()
	if len(got) != 1 {
		t.Fatalf("%d emails", len(got))
	}
	body := text(got[0])
	for _, want := range []string{"Watch it join a server: https://www.tiktok.com/@queueup/video/1", "Got me in on wipe day"} {
		if !strings.Contains(body, want) {
			t.Errorf("email two is missing %q:\n%s", want, body)
		}
	}
}

// The unsubscribe endpoint: the link works, it lasts, and it stops the rest.
func TestTheUnsubscribeLinkWorksAndStopsTheRest(t *testing.T) {
	r := newWinbackRig(t, "", "")
	start := time.Now().Add(-3 * time.Hour)
	r.signup(t, "leave@example.com", "TIKTOK", 0, start)
	r.srv.sendWinback(context.Background(), start.Add(3*time.Hour))
	q := unsubscribeFrom(t, text(r.sent()[0]))

	post := func(a, tok string) int {
		b, _ := json.Marshal(map[string]string{"a": a, "t": tok})
		resp, err := http.Post(r.ts.URL+"/api/unsubscribe", "application/json", strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(q.Get("a"), "not-the-token"); code != http.StatusBadRequest {
		t.Errorf("a wrong token = %d, want 400", code)
	}
	if code := post(q.Get("a"), q.Get("t")); code != http.StatusOK {
		t.Fatalf("unsubscribing = %d", code)
	}
	if code := post(q.Get("a"), q.Get("t")); code != http.StatusOK {
		t.Errorf("unsubscribing twice = %d, want 200 again", code)
	}

	for _, later := range []time.Duration{25 * time.Hour, 69 * time.Hour} {
		r.srv.sendWinback(context.Background(), start.Add(later))
	}
	if n := len(r.sent()); n != 1 {
		t.Errorf("unsubscribed, and still got %d emails", n)
	}
}

// The offer's deadline is enforced at the checkout, on the server. After it,
// the code still records where they came from — that is what codes are for —
// but the first month is full price, and the paywall is told so.
func TestTheOfferEndsAfterSeventyTwoHours(t *testing.T) {
	cases := []struct {
		age      time.Duration
		discount bool
	}{
		{71 * time.Hour, true},
		{73 * time.Hour, false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v after signup", c.age), func(t *testing.T) {
			b := newBillingRig(t, true)
			if err := b.st.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`,
				time.Now().Add(-c.age).UnixMilli(), b.acct.ID); err != nil {
				t.Fatal(err)
			}

			code, out := b.callBody(t, "POST", "/api/billing/code", `{"code":"TIKTOK"}`)
			if code != 200 || out["valid"] != true {
				t.Fatalf("code check = %d %v", code, out)
			}
			wantPence := float64(199)
			if !c.discount {
				wantPence = 499
				if out["expired"] != true {
					t.Errorf("an ended offer was not reported as ended: %v", out)
				}
			}
			if out["first_month_pence"] != wantPence {
				t.Errorf("paywall told first month = %v, want %v", out["first_month_pence"], wantPence)
			}

			if code, out := b.callBody(t, "POST", "/api/billing/checkout", `{"code":"TIKTOK"}`); code != 200 {
				t.Fatalf("checkout = %d %v", code, out)
			}
			form := b.stripe.lastCheckout(t)
			if got := form.Get("discounts[0][promotion_code]") != ""; got != c.discount {
				t.Errorf("discount applied = %v, want %v", got, c.discount)
			}
			// Attribution survives the deadline either way.
			if src := form.Get("subscription_data[metadata][source]"); src != "TIKTOK" {
				t.Errorf("source = %q, want TIKTOK even after the offer ends", src)
			}

			_, bill := b.call(t, "GET", "/api/billing")
			if _, ok := bill["offer_ends_at"]; !ok {
				t.Error("the paywall has no deadline to count down to")
			}
		})
	}
}

// The time left, as every email says it: whole hours, to the nearest one.
func TestTimeLeftIsSaidInHours(t *testing.T) {
	// To the nearest hour: the usual send, just after the mark, says "4
	// hours", and a late one never overstates by more than half an hour.
	end := time.Date(2026, 10, 8, 13, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		left time.Duration
		want string
	}{
		{69*time.Hour + 55*time.Minute, "70 hours"}, // email one
		{48 * time.Hour, "48 hours"},                // email two
		{4 * time.Hour, "4 hours"},
		{3*time.Hour + 55*time.Minute, "4 hours"}, // the normal five-minute tick
		{3*time.Hour + 10*time.Minute, "3 hours"}, // never "4" for this
		{50 * time.Minute, "1 hour"},
		{10 * time.Minute, "1 hour"},
	} {
		if got := hoursLeft(end, end.Add(-c.left)); got != c.want {
			t.Errorf("%v left: %q, want %q", c.left, got, c.want)
		}
	}
}

// continueFrom pulls the "pick up where you left off" link out of an email.
func continueFrom(t *testing.T, body string) url.Values {
	t.Helper()
	i := strings.Index(body, "https://queueuprust.com/continue?")
	if i < 0 {
		t.Fatalf("no continue link in:\n%s", body)
	}
	u, err := url.Parse(strings.Fields(body[i:])[0])
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func (r *winbackRig) do(t *testing.T, method, path, session, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, r.ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *winbackRig) follow(t *testing.T, q url.Values) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"a": q.Get("a"), "t": q.Get("t"), "code": q.Get("promo")})
	return r.do(t, "POST", "/api/auth/continue", "", string(b))
}

// The link in the email, tapped somewhere they are not signed in: straight
// back to their own paywall, at the price the email promised, counting down
// from their own signup — not a fresh 72 hours, and not the full price.
func TestTheEmailLinkLandsOnTheirOwnOfferAndCountdown(t *testing.T) {
	r := newWinbackRig(t, "", "")
	start := time.Now().Add(-3 * time.Hour)
	a := r.signup(t, "tapped@example.com", "TIKTOK", 0, start)
	r.srv.sendWinback(context.Background(), time.Now())
	got := r.sent()
	if len(got) != 1 {
		t.Fatalf("%d emails", len(got))
	}
	q := continueFrom(t, text(got[0]))
	if q.Get("a") != a.ID || q.Get("t") == "" || q.Get("promo") != "TIKTOK" {
		t.Fatalf("link carries %v", q)
	}

	code, out := r.follow(t, q)
	if code != http.StatusOK || out["session_token"] == nil {
		t.Fatalf("following the link = %d %v", code, out)
	}
	session := out["session_token"].(string)

	// Their account, not a new one.
	if code, me := r.do(t, "GET", "/api/auth/me", session, ""); code != 200 || me["email"] != "tapped@example.com" {
		t.Fatalf("signed in as %v (%d)", me["email"], code)
	}
	// Their countdown: 72 hours from when they signed up.
	_, bill := r.do(t, "GET", "/api/billing", session, "")
	ends, err := time.Parse(time.RFC3339Nano, fmt.Sprint(bill["offer_ends_at"]))
	if err != nil {
		t.Fatalf("offer_ends_at = %v", bill["offer_ends_at"])
	}
	if want := a.CreatedAt.Add(offerWindow); ends.Sub(want).Abs() > time.Second {
		t.Errorf("countdown ends %v, want %v (72 hours from their own signup)", ends, want)
	}
	// Their price.
	if _, chk := r.do(t, "POST", "/api/billing/code", session, `{"code":"TIKTOK"}`); chk["first_month_pence"] != float64(199) {
		t.Errorf("price on arrival = %v, want 199", chk["first_month_pence"])
	}
}

// The link is no stronger than typing the address: it never opens an account
// that has a password or a payment behind it.
func TestTheEmailLinkIsNoStrongerThanTypingTheEmail(t *testing.T) {
	setup := func(t *testing.T) (*winbackRig, store.Account, url.Values) {
		r := newWinbackRig(t, "", "")
		start := time.Now().Add(-3 * time.Hour)
		a := r.signup(t, "link@example.com", "TIKTOK", 0, start)
		r.srv.sendWinback(context.Background(), time.Now())
		return r, a, continueFrom(t, text(r.sent()[0]))
	}

	t.Run("after choosing a password", func(t *testing.T) {
		r, a, q := setup(t)
		if err := r.st.SetFirstPassword(a.ID, "a good password"); err != nil {
			t.Fatal(err)
		}
		code, out := r.follow(t, q)
		if code != http.StatusConflict || out["session_token"] != nil {
			t.Errorf("= %d %v, want 409 and no session", code, out)
		}
	})
	t.Run("after paying", func(t *testing.T) {
		r, a, q := setup(t)
		if err := r.st.SetSubscription(a.ID, "active", "sub_1"); err != nil {
			t.Fatal(err)
		}
		code, out := r.follow(t, q)
		if code != http.StatusAccepted || out["session_token"] != nil {
			t.Errorf("= %d %v, want 202 and no session", code, out)
		}
	})
	t.Run("a made-up token, or the unsubscribe one", func(t *testing.T) {
		r, a, q := setup(t)
		unsub := unsubscribeFrom(t, text(r.sent()[0])).Get("t")
		for _, tok := range []string{"made-up", unsub, ""} {
			bad := url.Values{"a": {a.ID}, "t": {tok}, "promo": {q.Get("promo")}}
			if code, out := r.follow(t, bad); code != http.StatusBadRequest || out["session_token"] != nil {
				t.Errorf("token %q = %d %v, want 400", tok, code, out)
			}
		}
	})
}

// The day-25 warning before the first full-price month.
//
// The paywall promises it in as many words, so it has to go — once, to the
// right people, at the right time. A renewal nobody saw coming is a
// chargeback, which costs the fee, the customer and a mark against every
// payment after it.
func TestTheRenewalWarningGoesOnceBeforeTheFullPriceMonth(t *testing.T) {
	r := newWinbackRig(t, "", "")
	now := time.Now()

	// paid 25 days ago and still paying: this is the one.
	due := r.signup(t, "renewing@example.com", "TIKTOK", 26*24*time.Hour, now)
	// paid only 10 days ago: too soon.
	early := r.signup(t, "new@example.com", "TIKTOK", 11*24*time.Hour, now)
	// already cancelled: they know, and are not being charged again.
	leaving := r.signup(t, "leaving@example.com", "TIKTOK", 26*24*time.Hour, now)
	// never paid: the win-back emails are their story, not this one.
	unpaid := r.signup(t, "browsing@example.com", "TIKTOK", 26*24*time.Hour, now)

	for _, a := range []store.Account{due, early, leaving} {
		if err := r.st.SetSubscription(a.ID, "active", "sub_"+a.ID); err != nil {
			t.Fatal(err)
		}
		if err := r.st.ExecForTests(
			`UPDATE accounts SET first_paid_at = (SELECT created_at FROM accounts WHERE id = ?) WHERE id = ?`,
			a.ID, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.st.NoteEnding(leaving.ID, now.Add(4*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	r.srv.sendRenewalReminders(context.Background(), now)
	r.srv.sendRenewalReminders(context.Background(), now.Add(time.Hour)) // and again

	got := r.sent()
	if len(got) != 1 {
		var to []any
		for _, e := range got {
			to = append(to, e["to"])
		}
		t.Fatalf("sent %d reminders to %v, want exactly one to %s", len(got), to, due.Email)
	}
	if addr, _ := got[0]["to"].([]any); len(addr) != 1 || addr[0] != due.Email {
		t.Errorf("reminder went to %v", got[0]["to"])
	}
	body := text(got[0])
	for _, want := range []string{"five days", "£4.99", "cancel in two taps", "/settings"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			t.Errorf("the reminder does not mention %q:\n%s", want, body)
		}
	}
	_ = unpaid
}
