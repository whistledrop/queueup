package game

import (
	"strings"
	"testing"
	"time"
)

// Steam paused the Rust download. StateFlags 1538 is update-required plus
// update-paused. This is what a full disk, a Steam set to offline, or a
// download somebody paused by accident all look like from outside.
const manifestPaused = `"AppState"
{
	"appid"		"252490"
	"name"		"Rust"
	"StateFlags"		"1538"
	"installdir"		"Rust"
	"BytesToDownload"		"4294967296"
	"BytesDownloaded"		"1073741824"
}`

func TestPausedDownloadIsRecognisedAsPaused(t *testing.T) {
	st := parseAppManifest(manifestPaused)
	if !st.Updating {
		t.Fatal("a paused update is still an update in progress")
	}
	if !st.Paused {
		t.Fatal("a paused download must be distinguishable from a running one")
	}
	// The player has to be told to go and do something, because unlike a normal
	// download this one will never finish on its own.
	msg := st.Describe()
	for _, want := range []string{"paused", "Steam"} {
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(want)) {
			t.Errorf("message %q does not mention %q", msg, want)
		}
	}
	if strings.Contains(msg, "as soon as it finishes") {
		t.Errorf("a paused download must not promise it will finish: %q", msg)
	}
}

func TestRunningDownloadIsNotReportedAsPaused(t *testing.T) {
	if parseAppManifest(manifestUpdating).Paused {
		t.Error("an update that is downloading was reported as paused")
	}
	if parseAppManifest(manifestInstalled).Paused {
		t.Error("an installed game was reported as paused")
	}
}

// A download that is not moving must eventually be called out. Without this the
// agent waits forever, because "Steam is updating" refreshes its patience on
// every poll, and the phone sits on "0% of 4.0 GB" until the player gives up.
func TestStalledDownloadIsDetected(t *testing.T) {
	clock := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	w := newStallWatch(func() time.Time { return clock })

	downloading := UpdateState{Known: true, Updating: true, BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30}

	if w.stalledFor(downloading) != 0 {
		t.Fatal("the first sighting of a download cannot be stalled")
	}

	// Progress: the clock moves and so do the bytes.
	clock = clock.Add(5 * time.Minute)
	downloading.BytesDownloaded = 2 << 30
	if d := w.stalledFor(downloading); d != 0 {
		t.Fatalf("a download that gained a gigabyte is stalled for %s", d)
	}

	// Now it stops moving.
	clock = clock.Add(4 * time.Minute)
	if d := w.stalledFor(downloading); d != 4*time.Minute {
		t.Fatalf("stalled for %s, want 4m", d)
	}
	clock = clock.Add(6 * time.Minute)
	if d := w.stalledFor(downloading); d != 10*time.Minute {
		t.Fatalf("stalled for %s, want 10m", d)
	}

	// It starts moving again, and the stall clock resets.
	downloading.BytesDownloaded = 3 << 30
	if d := w.stalledFor(downloading); d != 0 {
		t.Fatalf("stalled for %s after progress resumed, want 0", d)
	}
}

func TestStallWatchResetsWhenTheUpdateEnds(t *testing.T) {
	clock := time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
	w := newStallWatch(func() time.Time { return clock })

	updating := UpdateState{Known: true, Updating: true, BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30}
	w.stalledFor(updating)
	clock = clock.Add(20 * time.Minute)
	if w.stalledFor(updating) == 0 {
		t.Fatal("setup: expected a stall")
	}

	// The update finishes. A later update must start from a clean slate rather
	// than inheriting the previous one's stall.
	w.stalledFor(UpdateState{Known: true, Installed: true})
	clock = clock.Add(time.Minute)
	next := UpdateState{Known: true, Updating: true, BytesToDownload: 8 << 30, BytesDownloaded: 0}
	if d := w.stalledFor(next); d != 0 {
		t.Fatalf("a fresh update inherited a stall of %s", d)
	}
}

// The whole point of noticing: the message has to change from reassuring to
// actionable, because these two situations need different things from the
// player. One needs patience, the other needs them to go and look at the PC.
func TestStalledMessageIsActionable(t *testing.T) {
	st := UpdateState{
		Known: true, Updating: true,
		BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30,
		StalledFor: 12 * time.Minute,
	}
	msg := st.Describe()
	if strings.Contains(msg, "as soon as it finishes") {
		t.Errorf("a stalled download must not promise it will finish: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not moving") &&
		!strings.Contains(strings.ToLower(msg), "stopped") {
		t.Errorf("message %q does not say the download has stopped", msg)
	}
	if !strings.Contains(msg, "Steam") {
		t.Errorf("message %q does not tell the player where to look", msg)
	}
}
