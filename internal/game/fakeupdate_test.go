package game

import (
	"strings"
	"testing"
)

// The pretend state must be off unless it is asked for. A test hook that leaks
// into a real player's wipe day would be worse than not having one.
func TestThePretendUpdateIsOffByDefault(t *testing.T) {
	t.Setenv(FakeUpdateEnv, "")
	if u, ok := FakeUpdate(); ok {
		t.Fatalf("a pretend update was active with the variable unset: %+v", u)
	}
	// Anything unrecognised is also off, rather than quietly meaning something.
	t.Setenv(FakeUpdateEnv, "yes-please")
	if _, ok := FakeUpdate(); ok {
		t.Fatal("an unrecognised value switched the pretend update on")
	}
	if got := RustUpdateState(); got.Known {
		t.Fatalf("RustUpdateState invented a state from a bad value: %+v", got)
	}
}

// Each mode has to produce the sentence the player would really see, and the
// right answer to "can waiting fix this?". That second question is the one that
// decides whether the job waits patiently or gives up and explains itself.
func TestEachPretendUpdateReadsLikeTheRealThing(t *testing.T) {
	cases := []struct {
		value       string
		needsPlayer bool
		says        string
		verdict     launchVerdict
	}{
		{"moving", false, "Steam is updating Rust", verdictExtendGrace},
		{"paused", true, "paused the Rust download", verdictGiveUpBlaming},
		{"stalled", true, "stopped downloading Rust", verdictGiveUpBlaming},
	}

	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			t.Setenv(FakeUpdateEnv, c.value)

			u, ok := FakeUpdate()
			if !ok {
				t.Fatalf("%q did not switch the pretend update on", c.value)
			}
			if u.NeedsPlayer() != c.needsPlayer {
				t.Errorf("NeedsPlayer = %v, want %v", u.NeedsPlayer(), c.needsPlayer)
			}
			if got := u.Describe(); !strings.Contains(got, c.says) {
				t.Errorf("the phone would say %q, which does not contain %q", got, c.says)
			}
			// Past the deadline is the interesting moment: the game has not
			// appeared, and what happens next depends entirely on this state.
			if got := judgeLaunchWait(u, true); got != c.verdict {
				t.Errorf("verdict = %v, want %v", got, c.verdict)
			}
			// And it must reach callers through the ordinary entry point.
			if got := RustUpdateState(); got.Describe() != u.Describe() {
				t.Errorf("RustUpdateState did not return the pretend state")
			}
		})
	}
}

// A download that is moving buys unlimited patience even before the deadline,
// which is the behaviour force wipe depends on.
func TestAMovingDownloadIsNeverGivenUpOn(t *testing.T) {
	t.Setenv(FakeUpdateEnv, "moving")
	u, _ := FakeUpdate()
	if got := judgeLaunchWait(u, false); got != verdictExtendGrace {
		t.Errorf("verdict before the deadline = %v, want extend grace", got)
	}
	if got := judgeLaunchWait(u, true); got != verdictExtendGrace {
		t.Errorf("verdict past the deadline = %v, want extend grace: a download that is "+
			"working must never be given up on, however long it takes", got)
	}
}

// The simulator reports the pretend state too, which is what lets the whole
// thing be watched on a phone without a real game.
func TestTheSimulatorReportsThePretendUpdate(t *testing.T) {
	sim := &SimLauncher{}

	t.Setenv(FakeUpdateEnv, "")
	if got := sim.UpdateProgress().Describe(); got != "" {
		t.Errorf("the fake game claimed a Steam update with none set: %q", got)
	}

	t.Setenv(FakeUpdateEnv, "paused")
	if got := sim.UpdateProgress().Describe(); !strings.Contains(got, "paused") {
		t.Errorf("the fake game did not report the pretend update: %q", got)
	}
}
