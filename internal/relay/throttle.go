package relay

import (
	"sync"
	"time"
)

// throttle counts recent failures per key and blocks once there are too many.
//
// Two reasons this exists, and the second one is the reason it exists NOW.
//
// The obvious one is guessing passwords: without a limit, an attacker can sit
// there trying, and QueueUp accounts can start somebody's PC.
//
// The one that bites on wipe day is cost. Checking a password is deliberately
// expensive, which is what makes stolen password files useless, but it also
// means every sign-in attempt costs the relay real work. The relay is one small
// virtual machine. Somebody hammering the sign-in page, deliberately or through
// a broken script, could eat the processor that everybody else's join is
// waiting on, at the worst possible moment.
type throttle struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	fails map[string][]time.Time
}

func newThrottle(limit int, window time.Duration, now func() time.Time) *throttle {
	if now == nil {
		now = time.Now
	}
	return &throttle{limit: limit, window: window, now: now, fails: map[string][]time.Time{}}
}

// blocked reports whether this key has failed too often lately.
func (t *throttle) blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.recent(key)) >= t.limit
}

// fail records one failure.
func (t *throttle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fails[key] = append(t.recent(key), t.now())
	t.sweep()
}

// reset forgets a key, which is what a successful sign-in earns.
func (t *throttle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, key)
}

func (t *throttle) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweep()
	return len(t.fails)
}

// recent returns the failures still inside the window. Caller holds the lock.
func (t *throttle) recent(key string) []time.Time {
	cutoff := t.now().Add(-t.window)
	kept := t.fails[key][:0]
	for _, at := range t.fails[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	t.fails[key] = kept
	return kept
}

// sweep drops keys whose failures have all aged out, so a stream of mistyped
// addresses cannot grow the map forever. Caller holds the lock.
func (t *throttle) sweep() {
	cutoff := t.now().Add(-t.window)
	for key, times := range t.fails {
		if len(times) == 0 || !times[len(times)-1].After(cutoff) {
			delete(t.fails, key)
		}
	}
}
