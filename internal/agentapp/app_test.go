package agentapp

import (
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/protocol"
	"queueup/internal/relayclient"
	"queueup/internal/scenario"
)

// statusRecorder captures everything the agent would have told the relay.
type statusRecorder struct {
	mu   sync.Mutex
	sent []protocol.JobStatus
}

func (r *statusRecorder) states() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.sent))
	for _, s := range r.sent {
		out = append(out, s.State)
	}
	return out
}

func (r *statusRecorder) last() (protocol.JobStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sent) == 0 {
		return protocol.JobStatus{}, false
	}
	return r.sent[len(r.sent)-1], true
}

func newTestApp(t *testing.T, scenarioName string) (*App, *game.SimLauncher, *statusRecorder) {
	t.Helper()
	sc, err := scenario.Load(filepath.Join("../../testdata/scenarios", scenarioName+".json"))
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	launcher := &game.SimLauncher{
		Scenario: sc,
		Log:      filepath.Join(t.TempDir(), "Player.log"),
		Speed:    20,
	}
	rec := &statusRecorder{}
	client := &relayclient.Client{Log: quiet}
	client.CaptureForTests(func(tp protocol.Type, payload any) {
		if tp != protocol.TypeJobStatus {
			return
		}
		if st, ok := payload.(protocol.JobStatus); ok {
			rec.mu.Lock()
			rec.sent = append(rec.sent, st)
			rec.mu.Unlock()
		}
	})

	app := &App{
		Client: client, Parser: parser, Log: quiet,
		NewGame: func(protocol.Job) (game.Launcher, error) { return launcher, nil },
		JobConfig: job.Config{
			InServerConfirm: 300 * time.Millisecond,
			RetryBase:       200 * time.Millisecond,
			RetryMax:        200 * time.Millisecond,
			MaxAttempts:     3,
		},
	}
	return app, launcher, rec
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The product's whole promise is "your PC holds the slot". When a join succeeds,
// Rust must be LEFT RUNNING. This is the full agent path, not just the runner:
// the bug this guards against was the agent closing the game it had just won
// with, one line after the machine said done.
func TestASuccessfulJoinLeavesTheGameRunning(t *testing.T) {
	app, launcher, rec := newTestApp(t, "instant_join")

	app.OnAssign(protocol.Job{ID: "job-hold", ServerAddr: "51.83.128.10:28015"})

	waitFor(t, "the join to finish", func() bool {
		st, ok := rec.last()
		return ok && st.State == "done"
	})
	// Any close that should not happen gets a moment to happen.
	time.Sleep(300 * time.Millisecond)

	if !launcher.Running() {
		t.Fatalf("the game was closed after a successful join; the slot is gone. states: %v", rec.states())
	}
	_ = launcher.Close()
}

// A FAILED job is the opposite: the game must not be left running with nobody
// managing it, or a rejected account would sit on the menu screen forever.
func TestAFailedJoinDoesCloseTheGame(t *testing.T) {
	app, launcher, rec := newTestApp(t, "rejected")

	app.OnAssign(protocol.Job{ID: "job-fail", ServerAddr: "51.83.128.10:28015"})

	waitFor(t, "the join to fail", func() bool {
		st, ok := rec.last()
		return ok && st.State == "failed"
	})
	waitFor(t, "the game to be closed", func() bool { return !launcher.Running() })
}
