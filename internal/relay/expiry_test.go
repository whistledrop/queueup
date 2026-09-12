package relay

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"queueup/internal/store"
)

func TestJobHasExpired(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	const window = 6 * time.Hour

	old := store.Job{State: "pending", UpdatedAt: now.Add(-12 * 24 * time.Hour)}
	recent := store.Job{State: "pending", UpdatedAt: now.Add(-10 * time.Minute)}
	waiting := store.Job{State: "waiting_for_server_up", UpdatedAt: now.Add(-5 * time.Hour)}
	finished := store.Job{State: "done", UpdatedAt: now.Add(-12 * 24 * time.Hour)}

	cases := []struct {
		name   string
		job    store.Job
		online bool
		want   bool
	}{
		{"twelve days, PC gone", old, false, true},
		{"twelve days, but the PC is here", old, true, false},
		{"recent, PC gone", recent, false, false},
		{"already finished", finished, false, false},
		{"waiting for a wipe on a connected PC", waiting, true, false},
	}
	for _, c := range cases {
		if got := jobHasExpired(c.job, c.online, now, window); got != c.want {
			t.Errorf("%s: expired = %v, want %v", c.name, got, c.want)
		}
	}
}

// The case that matters most: a job quietly waiting for a wipe restart, on a PC
// that is present, must survive however long the wait. Expiring that would
// break the feature the product exists for.
func TestAQuietWipeWaitOnAConnectedPCIsNeverExpired(t *testing.T) {
	now := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	j := store.Job{State: "waiting_for_server_up", UpdatedAt: now.Add(-23 * time.Hour)}
	if jobHasExpired(j, true, now, DefaultJobExpiry) {
		t.Fatal("a wipe-day wait on a live PC was thrown away for being quiet")
	}
}

// End to end through the store: a stale job is closed with a reason a person
// can act on, and the timeline records it.
func TestSweepClosesAStaleJobWithAPlainReason(t *testing.T) {
	st := watcherStore(t)
	acct, _, err := st.CreateAccount("stale@example.com")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.StartPairing("Gone PC")
	if err != nil {
		t.Fatal(err)
	}
	d, err := st.ClaimPairingCode(acct.ID, p.Code)
	if err != nil {
		t.Fatal(err)
	}
	j, err := st.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015",
	})
	if err != nil {
		t.Fatal(err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{st: st, log: quiet, hub: NewHub(quiet)}

	// Nothing has been offline long enough yet.
	s.sweepExpiredJobs(6 * time.Hour)
	if got, _ := st.JobByID(j.ID); !got.Active() {
		t.Fatal("a job created moments ago was expired")
	}

	// A window of zero would disable it, so use a negative-age window instead:
	// sweep with a window small enough that the job counts as stale.
	s.sweepExpiredJobs(time.Nanosecond)
	got, err := st.JobByID(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active() {
		t.Fatal("a stale job on an absent PC was left open")
	}
	if got.ReasonCode != "expired" {
		t.Errorf("reason code = %q, want expired", got.ReasonCode)
	}
	if len(got.ReasonMessage) < 20 {
		t.Errorf("reason %q is too terse to show a player", got.ReasonMessage)
	}
	events, _ := st.Events(j.ID, 0)
	if len(events) < 2 {
		t.Errorf("the timeline does not record the job being closed: %+v", events)
	}
}
