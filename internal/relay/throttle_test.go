package relay

import (
	"testing"
	"time"
)

func TestThrottleBlocksAfterRepeatedFailures(t *testing.T) {
	now := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	th := newThrottle(5, 15*time.Minute, func() time.Time { return now })

	for i := 0; i < 4; i++ {
		th.fail("someone@example.com")
		if th.blocked("someone@example.com") {
			t.Fatalf("blocked after only %d failures", i+1)
		}
	}
	th.fail("someone@example.com")
	if !th.blocked("someone@example.com") {
		t.Fatal("still allowed after hitting the limit")
	}
	// Somebody else is unaffected.
	if th.blocked("innocent@example.com") {
		t.Fatal("one account's failures blocked a different account")
	}
}

func TestThrottleForgetsOldFailures(t *testing.T) {
	now := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	th := newThrottle(3, 15*time.Minute, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		th.fail("someone@example.com")
	}
	if !th.blocked("someone@example.com") {
		t.Fatal("setup: expected a block")
	}

	// A locked-out player must get back in without support: the window passes
	// and the slate is clean.
	now = now.Add(16 * time.Minute)
	if th.blocked("someone@example.com") {
		t.Fatal("still locked out long after the window passed")
	}
}

func TestASuccessfulSignInClearsTheSlate(t *testing.T) {
	now := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	th := newThrottle(3, 15*time.Minute, func() time.Time { return now })

	th.fail("someone@example.com")
	th.fail("someone@example.com")
	th.reset("someone@example.com")

	for i := 0; i < 2; i++ {
		th.fail("someone@example.com")
	}
	if th.blocked("someone@example.com") {
		t.Fatal("old failures counted after a successful sign in")
	}
}

// The map must not grow without bound just because people mistype.
func TestThrottleForgetsKeysItNoLongerNeeds(t *testing.T) {
	now := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	th := newThrottle(3, time.Minute, func() time.Time { return now })

	for i := 0; i < 500; i++ {
		th.fail(string(rune('a'+i%26)) + time.Duration(i).String())
	}
	now = now.Add(2 * time.Minute)
	th.fail("someone-new@example.com")

	if n := th.size(); n > 10 {
		t.Fatalf("throttle is holding %d stale keys long after their window passed", n)
	}
}
