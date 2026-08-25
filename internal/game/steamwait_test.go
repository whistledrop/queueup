package game

import (
	"testing"
	"time"
)

// A fake clock that only moves when the code under test sleeps, so ninety
// seconds of patience takes no real time to verify.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time        { return c.t }
func (c *fakeClock) pause(d time.Duration) { c.t = c.t.Add(d) }
func newFakeClock() *fakeClock             { return &fakeClock{t: time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)} }

func TestAwaitProcessReturnsAtOnceWhenAlreadyRunning(t *testing.T) {
	c := newFakeClock()
	calls := 0
	ok := awaitProcess(func() bool { calls++; return true }, SteamRestartGrace, c.now, c.pause)
	if !ok {
		t.Fatal("a running process was not seen")
	}
	if calls != 1 {
		t.Errorf("checked %d times, want 1: the normal case must not wait at all", calls)
	}
	if !c.now().Equal(newFakeClock().now()) {
		t.Error("time passed while the process was already there")
	}
}

// Steam restarting mid-update: gone for a while, then back.
func TestAwaitProcessWaitsThroughARestart(t *testing.T) {
	c := newFakeClock()
	start := c.now()
	back := start.Add(30 * time.Second)

	ok := awaitProcess(func() bool { return !c.now().Before(back) }, SteamRestartGrace, c.now, c.pause)
	if !ok {
		t.Fatal("Steam came back within the grace period but was reported missing")
	}
	if waited := c.now().Sub(start); waited > 35*time.Second {
		t.Errorf("waited %s for a process that returned after 30s", waited)
	}
}

func TestAwaitProcessGivesUpEventually(t *testing.T) {
	c := newFakeClock()
	start := c.now()
	if awaitProcess(func() bool { return false }, SteamRestartGrace, c.now, c.pause) {
		t.Fatal("a process that never appeared was reported as running")
	}
	waited := c.now().Sub(start)
	if waited < SteamRestartGrace {
		t.Errorf("gave up after %s, before the grace period was up", waited)
	}
	if waited > SteamRestartGrace+5*time.Second {
		t.Errorf("waited %s, well past the grace period", waited)
	}
}
