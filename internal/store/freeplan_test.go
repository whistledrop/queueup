package store_test

import (
	"testing"
	"time"

	"queueup/internal/store"
)

// What the free plan charges for, and what it deliberately does not.
//
// The rule is "the player started it AND it worked". Everything else is free,
// because everything else is either our fault or their change of mind.
func TestOnlySuccessfulJoinsCountAgainstTheFreeAllowance(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	since := time.Now().Add(-time.Hour)

	// Finish a job the way the agent would, and say whether it was a win.
	join := func(state, reasonCode string) {
		t.Helper()
		j, err := s.CreateJob(store.NewJob{
			AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.FinishJob(j.ID, state, reasonCode, ""); err != nil {
			t.Fatal(err)
		}
	}

	// Two that worked. A win is "done" with no reason: the state machine only
	// leaves a reason behind when something happened TO the join.
	join("done", "")
	join("done", "")

	// None of these cost the player anything.
	join("done", "cancelled")      // they changed their mind
	join("done", "player_closed")  // they closed Rust themselves
	join("done", "player_left")    // they left the queue at the keyboard
	join("done", "superseded")     // a scheduled join took over
	join("failed", "server_full")  // the server said no
	join("failed", "game_crashed") // Rust fell over
	join("failed", "gave_up")      // we ran out of attempts
	join("failed", "steam_problem")

	got, err := s.SuccessfulJoinsSince(acct.ID, since)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("counted %d joins, want 2: only the two that actually got in", got)
	}
}

// A join that retried, reconnected or rejoined after a crash is still ONE
// join. All of that happens inside a single job row, so there is nothing to
// subtract — this test exists to catch anyone who later "fixes" the count by
// looking at attempts.
func TestRetriesInsideOneJoinAreStillOneJoin(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	since := time.Now().Add(-time.Hour)

	j, err := s.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The agent fights with it for a while: launch, retry, retry, then in.
	for _, st := range []string{"launching", "connecting", "retrying", "connecting", "queued", "in_server"} {
		if err := s.ExecForTests(`UPDATE jobs SET state = ?, attempt = attempt + 1 WHERE id = ?`, st, j.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishJob(j.ID, "done", "", ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.SuccessfulJoinsSince(acct.ID, since)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("counted %d, want 1: retries are one join, not several", got)
	}
}

// A teammate pulled into somebody else's squad join does not pay for it out of
// their own ten. Squad joins can only be started by a paying member, so a job
// carrying a group id was always paid for by somebody.
func TestSquadJoinsDoNotCostTheTeammateAJoin(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	since := time.Now().Add(-time.Hour)

	own, err := s.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
	})
	if err != nil {
		t.Fatal(err)
	}
	squad, err := s.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
		GroupID: "squadjoin_123",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{own.ID, squad.ID} {
		if err := s.FinishJob(id, "done", "", ""); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.SuccessfulJoinsSince(acct.ID, since)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("counted %d, want 1: the squad join was somebody else's to pay for", got)
	}
}

// The allowance is a calendar month, and last month's joins are last month's.
func TestTheAllowanceIsACalendarMonth(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	j, err := s.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishJob(j.ID, "done", "", ""); err != nil {
		t.Fatal(err)
	}
	// Backdate it into last month.
	lastMonth := store.MonthStart(time.Now().UTC()).AddDate(0, 0, -1)
	if err := s.ExecForTests(`UPDATE jobs SET created_at = ? WHERE id = ?`,
		lastMonth.UnixMilli(), j.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.SuccessfulJoinsSince(acct.ID, store.MonthStart(time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("counted %d, want 0: the 1st wipes the slate", got)
	}
}

// The one free wipe join is once, ever, and cannot be handed out twice.
func TestTheFreeWipeJoinIsSpentOnlyOnce(t *testing.T) {
	s := newStore(t)
	acct, _ := pairedDevice(t, s)

	at, err := s.FreeWipeJoinUsedAt(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !at.IsZero() {
		t.Fatalf("a new account has already spent its free wipe join: %v", at)
	}

	first := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.MarkFreeWipeJoinUsed(acct.ID, first); err != nil {
		t.Fatal(err)
	}
	// A second attempt must not move the date or hand out another.
	if err := s.MarkFreeWipeJoinUsed(acct.ID, first.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.FreeWipeJoinUsedAt(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(first) {
		t.Errorf("spent at %v, want %v: spending it twice must change nothing", got, first)
	}
}
