package relay

import (
	"strings"
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

	// name and report say out loud when this throttle turns somebody away.
	// It used to be silent, which is how every signup in the world sharing
	// one Netlify address went unnoticed: people were being refused and
	// nothing anywhere said so.
	name   string
	report func(name, key string)

	mu    sync.Mutex
	fails map[string][]time.Time
	// noted is when each key was last reported, so one source hammering away
	// is one line in the log per window, not one per attempt.
	noted map[string]time.Time
}

func newThrottle(limit int, window time.Duration, now func() time.Time) *throttle {
	if now == nil {
		now = time.Now
	}
	return &throttle{limit: limit, window: window, now: now,
		fails: map[string][]time.Time{}, noted: map[string]time.Time{}}
}

// blocked reports whether this key has failed too often lately.
func (t *throttle) blocked(key string) bool {
	t.mu.Lock()
	over := len(t.recent(key)) >= t.limit
	tell := false
	if over && t.report != nil {
		if last, ok := t.noted[key]; !ok || t.now().Sub(last) > t.window {
			t.noted[key] = t.now()
			tell = true
		}
	}
	t.mu.Unlock()
	if tell {
		t.report(t.name, redactKey(key))
	}
	return over
}

// redactKey keeps email addresses out of the log. An address on the internet
// is what the limits are about and is worth seeing; whose account was being
// guessed at is not something a log needs to keep.
func redactKey(key string) string {
	if !strings.Contains(key, "@") {
		return key
	}
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i+1] + "(an email address)"
	}
	return "(an email address)"
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
	for key, at := range t.noted {
		if !at.After(cutoff) {
			delete(t.noted, key)
		}
	}
}
