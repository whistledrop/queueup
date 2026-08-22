package job

import (
	"testing"
	"time"
)

// testClock lets every timing case run instantly and identically every time.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestMachine(cfg Config) (*Machine, *testClock) {
	c := &testClock{t: time.Date(2026, 8, 22, 18, 0, 0, 0, time.UTC)}
	m := New(cfg,
		WithClock(c.now),
		WithJitter(func(max time.Duration) time.Duration { return max / 2 }), // deterministic
	)
	return m, c
}

// feed pushes inputs in and returns the states we passed through.
func feed(m *Machine, ins ...Input) []State {
	var states []State
	for _, in := range ins {
		for _, tr := range m.Handle(in).Transitions {
			states = append(states, tr.To)
		}
	}
	return states
}

func actionsFor(m *Machine, in Input) []Action { return m.Handle(in).Actions }

func TestHappyPathThroughQueue(t *testing.T) {
	m, c := newTestMachine(Config{InServerConfirm: 30 * time.Second})

	if got := m.State(); got != StateIdle {
		t.Fatalf("start state = %s, want idle", got)
	}
	if acts := actionsFor(m, Start{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("Start should ask the runner to launch the game, got %v", acts)
	}
	if m.State() != StateLaunching {
		t.Fatalf("state = %s, want launching", m.State())
	}

	feed(m, LaunchOK{})
	if m.State() != StateConnecting {
		t.Fatalf("state = %s, want connecting", m.State())
	}

	feed(m, LogEvent{Kind: "queued", Position: 212, Detail: "In queue, position 212"})
	if m.State() != StateQueued || m.Position() != 212 {
		t.Fatalf("state = %s position = %d, want queued/212", m.State(), m.Position())
	}

	feed(m, LogEvent{Kind: "queued", Position: 40})
	if m.Position() != 40 {
		t.Fatalf("position = %d, want 40", m.Position())
	}

	feed(m, LogEvent{Kind: "joined", Detail: "You're in."})
	if m.State() != StateInServer {
		t.Fatalf("state = %s, want in_server", m.State())
	}

	// Not done yet: we hold the slot for a bit before declaring success.
	feed(m, Tick{})
	if m.State() != StateInServer {
		t.Fatalf("job finished too early, before the confirm window elapsed")
	}
	c.advance(31 * time.Second)
	feed(m, Tick{})
	if m.State() != StateDone {
		t.Fatalf("state = %s, want done", m.State())
	}
}

func TestRepeatedSamePositionDoesNotSpamUpdates(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 100})
	res := m.Handle(LogEvent{Kind: "queued", Position: 100})
	if len(res.Transitions) != 0 {
		t.Fatalf("the same queue position produced %d updates; the phone would buzz for nothing", len(res.Transitions))
	}
}

func TestSteamProblemFailsImmediatelyWithAPlainReason(t *testing.T) {
	m, _ := newTestMachine(Config{MaxAttempts: 8})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "steam_problem", Detail: "Steam isn't logged in."})
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed (retrying cannot fix a logged-out Steam)", m.State())
	}
	if m.Failure() == nil || m.Failure().Code != "steam_problem" {
		t.Fatalf("failure = %+v, want a steam_problem reason", m.Failure())
	}
	if m.Attempt() != 1 {
		t.Fatalf("attempts = %d, want 1: we must not burn retries on this", m.Attempt())
	}
}

func TestRejectionIsNotRetried(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "rejected", Detail: "You are banned from this server."})
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed", m.State())
	}
	if m.Failure().Message != "You are banned from this server." {
		t.Fatalf("the user must be told the real reason, got %q", m.Failure().Message)
	}
}

func TestCrashMidQueueRelaunches(t *testing.T) {
	m, c := newTestMachine(Config{RetryBase: 5 * time.Second})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 240})

	feed(m, GameExited{Code: 1})
	if m.State() != StateRetrying {
		t.Fatalf("state = %s, want retrying after a crash", m.State())
	}
	if m.Position() != 240 {
		t.Log("note: position is retained until the relaunch, which is what the phone shows")
	}

	// Too early: still backing off.
	feed(m, Tick{})
	if m.State() != StateRetrying {
		t.Fatalf("relaunched before the backoff elapsed")
	}
	c.advance(6 * time.Second)
	if acts := actionsFor(m, Tick{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("expected a relaunch after the backoff, got %v", acts)
	}
	if m.Attempt() != 2 {
		t.Fatalf("attempts = %d, want 2", m.Attempt())
	}
	if m.Position() != 0 {
		t.Fatalf("queue position should reset on relaunch, got %d", m.Position())
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	m, _ := newTestMachine(Config{RetryBase: 5 * time.Second, RetryMax: 20 * time.Second})
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 20 * time.Second}
	for i, w := range want {
		m.attempt = i + 1
		if got := m.backoff(); got != w {
			t.Errorf("attempt %d backoff = %s, want %s", i+1, got, w)
		}
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 3, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{})
	for i := 0; i < 10 && m.State() != StateFailed; i++ {
		feed(m, LaunchOK{}, GameExited{Code: 1})
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed after running out of attempts", m.State())
	}
	if m.Failure().Code != "gave_up" {
		t.Fatalf("failure code = %s, want gave_up", m.Failure().Code)
	}
	if m.Attempt() > 3 {
		t.Fatalf("made %d attempts, cap was 3", m.Attempt())
	}
}

// Wipe day: the server bounces up and down before it settles. We must not
// launch on a false start, and we must not give up.
func TestServerFlappingDoesNotBurnAttempts(t *testing.T) {
	m, c := newTestMachine(Config{WaitForServerUp: true, ConnectJitterMax: 4 * time.Second})
	feed(m, Start{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s, want waiting_for_server_up", m.State())
	}

	// Up, then down again before our (jitter/2 = 2s) delay elapses.
	feed(m, ServerUp{})
	c.advance(1 * time.Second)
	feed(m, Tick{})
	feed(m, ServerDown{})
	c.advance(5 * time.Second)
	feed(m, Tick{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s: a false start launched the game", m.State())
	}
	if m.Attempt() != 0 {
		t.Fatalf("attempts = %d, want 0: flapping must not cost us a retry", m.Attempt())
	}

	// Now it comes up for real.
	feed(m, ServerUp{})
	c.advance(3 * time.Second)
	if acts := actionsFor(m, Tick{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("expected a launch once the server settled, got %v", acts)
	}
}

func TestConnectAttemptsAreRateLimited(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, MaxConnectsPerMinute: 2,
		ConnectJitterMax: 0, RetryBase: time.Second, RetryMax: time.Second, MaxAttempts: 20,
	})
	feed(m, Start{})
	launches := 0
	for i := 0; i < 10; i++ {
		feed(m, ServerUp{})
		res := m.Handle(Tick{})
		for _, a := range res.Actions {
			if a == ActionLaunchGame {
				launches++
			}
		}
		if m.State() == StateLaunching {
			feed(m, LaunchOK{}, GameExited{Code: 1}) // instant death, straight back round
		}
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}
	if launches > 2 {
		t.Fatalf("launched %d times inside a minute, cap was 2", launches)
	}
}

func TestCancelFromQueueClosesTheGame(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 5})
	res := m.Handle(Cancel{})
	if m.State() != StateDone {
		t.Fatalf("state = %s, want done", m.State())
	}
	found := false
	for _, a := range res.Actions {
		if a == ActionCloseGame {
			found = true
		}
	}
	if !found {
		t.Fatal("cancelling must close the game, otherwise Rust sits in the queue forever")
	}
}

func TestFinishedJobsIgnoreEverything(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "rejected"})
	if m.State() != StateFailed {
		t.Fatal("setup failed")
	}
	if res := m.Handle(LogEvent{Kind: "joined"}); len(res.Transitions) != 0 {
		t.Fatal("a finished job moved again")
	}
}
