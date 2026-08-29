package game

import (
	"testing"
	"time"
)

// The whole force-wipe launch story, as a table. This is the decision the agent
// makes every two seconds while Rust has not appeared yet, and each row is a
// real situation from the day that matters.
func TestJudgeLaunchWait(t *testing.T) {
	downloading := UpdateState{Known: true, Updating: true,
		BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30}
	paused := downloading
	paused.Paused = true
	stalled := downloading
	stalled.StalledFor = StallReport + time.Minute
	installed := UpdateState{Known: true, Installed: true}
	unknown := UpdateState{}

	cases := []struct {
		name         string
		update       UpdateState
		pastDeadline bool
		want         launchVerdict
	}{
		// Force wipe: Steam is pulling the multi-gigabyte update. However long
		// it takes, we stay patient. This is the case the product lives on.
		{"downloading, within patience", downloading, false, verdictExtendGrace},
		{"downloading, past the ordinary deadline", downloading, true, verdictExtendGrace},

		// The download stopped. Patience cannot fix a paused Steam or a full
		// disk, so the ordinary deadline applies, and when it passes the player
		// is told exactly what to go and look at.
		{"paused, within patience", paused, false, verdictKeepWaiting},
		{"paused, out of patience", paused, true, verdictGiveUpBlaming},
		{"stalled, within patience", stalled, false, verdictKeepWaiting},
		{"stalled, out of patience", stalled, true, verdictGiveUpBlaming},

		// No update in play: the normal slow EAC/Unity start. Wait, then give
		// up without inventing an explanation we do not have.
		{"installed, within patience", installed, false, verdictKeepWaiting},
		{"installed, out of patience", installed, true, verdictGiveUp},

		// Steam's manifest could not be found at all (odd install). Behave
		// exactly as if there were no update system: ordinary patience.
		{"unknown manifest, within patience", unknown, false, verdictKeepWaiting},
		{"unknown manifest, out of patience", unknown, true, verdictGiveUp},
	}
	for _, c := range cases {
		if got := judgeLaunchWait(c.update, c.pastDeadline); got != c.want {
			t.Errorf("%s: verdict = %v, want %v", c.name, got, c.want)
		}
	}
}

// The full force-wipe sequence, end to end through the verdict: download runs
// long past the ordinary deadline, finishes, and only then does the ordinary
// clock start mattering again.
func TestForceWipeSequenceStaysPatientUntilTheDownloadEnds(t *testing.T) {
	u := UpdateState{Known: true, Updating: true, BytesToDownload: 4 << 30}

	// Twenty minutes of downloading, far past any ordinary deadline: every
	// single check must extend patience, never give up.
	for i := 0; i < 600; i++ {
		u.BytesDownloaded += 7 << 20
		if v := judgeLaunchWait(u, true); v != verdictExtendGrace {
			t.Fatalf("gave up %d checks into a healthy download (verdict %v)", i, v)
		}
	}

	// Download done, game installed, and Rust takes a normal minute to appear:
	// keep waiting within the refreshed grace.
	done := UpdateState{Known: true, Installed: true}
	if v := judgeLaunchWait(done, false); v != verdictKeepWaiting {
		t.Fatalf("after the update finished, verdict = %v, want keep waiting", v)
	}
}
