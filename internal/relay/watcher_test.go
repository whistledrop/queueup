package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"queueup/internal/a2s"
	"queueup/internal/protocol"
	"queueup/internal/store"
)

func watcherStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// waitingJobs creates n jobs, all waiting for the same server to come back,
// which is exactly what a popular server looks like minutes before force wipe.
var watchAccounts int

func waitingJobs(t *testing.T, st *store.Store, n int, addr string) {
	t.Helper()
	watchAccounts++
	acct, _, err := st.CreateAccount(fmt.Sprintf("watch%d@example.com", watchAccounts))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		p, err := st.StartPairing("PC")
		if err != nil {
			t.Fatal(err)
		}
		d, err := st.ClaimPairingCode(acct.ID, p.Code)
		if err != nil {
			t.Fatal(err)
		}
		j, err := st.CreateJob(store.NewJob{
			AccountID: acct.ID, DeviceID: d.ID,
			ServerAddr: addr, WaitForServerUp: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := st.ApplyStatus(protocol.JobStatus{
			JobID: j.ID, State: "waiting_for_server_up", At: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// The wipe-restart detector's whole promise is that it spots the server coming
// back within seconds. A server that is DOWN does not refuse a query, it simply
// does not answer, so every poll costs the full timeout. If those polls happen
// one after another, the sweep takes jobs x timeout, and on the one day this
// product exists for, with lots of people waiting on the same popular server,
// the detector is minutes late.
func TestSweepDoesNotSlowDownAsJobsPileUp(t *testing.T) {
	st := watcherStore(t)
	const jobs = 12
	const queryCost = 300 * time.Millisecond
	waitingJobs(t, st, jobs, "51.83.128.10:28015")

	w := &Watcher{
		Store: st, Hub: NewHub(slog.New(slog.NewTextHandler(io.Discard, nil))),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		WaitingPoll: time.Millisecond, OtherPoll: time.Millisecond,
		Query: func(ctx context.Context, addr string) (a2s.Info, error) {
			// A server that is down: no reply until the timeout.
			select {
			case <-time.After(queryCost):
			case <-ctx.Done():
			}
			return a2s.Info{}, context.DeadlineExceeded
		},
	}
	w.prepare()

	start := time.Now()
	w.sweep(context.Background())
	took := time.Since(start)

	// Serial polling would take jobs x queryCost. Allow generous slack, but
	// nothing like the serial cost.
	if worst := jobs * queryCost; took > worst/3 {
		t.Fatalf("a sweep of %d jobs took %s; serial polling would be %s. "+
			"The restart detector cannot keep its promise at this speed.",
			jobs, took.Round(time.Millisecond), worst)
	}
}

// Everybody waiting on the same server is the normal wipe-day shape. Asking
// that one server the same question once per waiting player is both slow and
// rude, and a server being hammered by its own queue tool is a good way to get
// QueueUp blocked.
func TestOneServerIsAskedOncePerSweepHoweverManyPlayersAreWaiting(t *testing.T) {
	st := watcherStore(t)
	waitingJobs(t, st, 10, "51.83.128.10:28015")

	var mu sync.Mutex
	calls := map[string]int{}

	w := &Watcher{
		Store: st, Hub: NewHub(slog.New(slog.NewTextHandler(io.Discard, nil))),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		WaitingPoll: time.Millisecond, OtherPoll: time.Millisecond,
		Query: func(ctx context.Context, addr string) (a2s.Info, error) {
			mu.Lock()
			calls[addr]++
			mu.Unlock()
			return a2s.Info{Players: 3, MaxPlayers: 200}, nil
		},
	}
	w.prepare()
	w.sweep(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if n := calls["51.83.128.10:28010"] + calls["51.83.128.10:28015"]; n != 1 {
		t.Fatalf("asked the same server %d times in one sweep, want 1", n)
	}
}

// Two different servers must still both be polled.
func TestDifferentServersAreEachPolled(t *testing.T) {
	st := watcherStore(t)
	waitingJobs(t, st, 2, "51.83.128.10:28015")
	waitingJobs(t, st, 2, "45.62.160.40:28015")

	var mu sync.Mutex
	seen := map[string]bool{}
	w := &Watcher{
		Store: st, Hub: NewHub(slog.New(slog.NewTextHandler(io.Discard, nil))),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		WaitingPoll: time.Millisecond, OtherPoll: time.Millisecond,
		Query: func(ctx context.Context, addr string) (a2s.Info, error) {
			mu.Lock()
			seen[addr] = true
			mu.Unlock()
			return a2s.Info{}, nil
		},
	}
	w.prepare()
	w.sweep(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("polled %d distinct servers, want 2: %v", len(seen), seen)
	}
}
