package game

import (
	"strings"
	"testing"
)

// A real appmanifest for a game that is installed and ready. StateFlags 4 is
// "fully installed".
const manifestInstalled = `"AppState"
{
	"appid"		"252490"
	"Universe"		"1"
	"name"		"Rust"
	"StateFlags"		"4"
	"installdir"		"Rust"
	"BytesToDownload"		"0"
	"BytesDownloaded"		"0"
}`

// Force wipe day: Steam is part way through downloading the update that ships
// with the wipe. StateFlags 1026 is update-required plus update-started.
const manifestUpdating = `"AppState"
{
	"appid"		"252490"
	"name"		"Rust"
	"StateFlags"		"1026"
	"installdir"		"Rust"
	"BytesToDownload"		"4294967296"
	"BytesDownloaded"		"1073741824"
}`

func TestManifestInstalledMeansReadyToPlay(t *testing.T) {
	st := parseAppManifest(manifestInstalled)
	if !st.Known {
		t.Fatal("a valid manifest was not recognised")
	}
	if !st.Installed {
		t.Error("StateFlags 4 should mean installed")
	}
	if st.Updating {
		t.Error("an installed game with nothing to download is not updating")
	}
	if st.Describe() != "" {
		t.Errorf("nothing should be shown to the player, got %q", st.Describe())
	}
}

// The case that matters: this must NOT look like a crash, and the player must
// be told what is happening rather than watching nothing.
func TestManifestUpdatingIsPatienceNotFailure(t *testing.T) {
	st := parseAppManifest(manifestUpdating)
	if !st.Known || !st.Updating {
		t.Fatalf("a mid-download manifest should read as updating: %+v", st)
	}
	if got := st.Percent(); got != 25 {
		t.Errorf("Percent() = %d, want 25 (1GB of 4GB)", got)
	}
	msg := st.Describe()
	for _, want := range []string{"updating Rust", "25%", "4.0 GB", "force wipe"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message %q does not mention %q", msg, want)
		}
	}
}

// A game that is not installed at all still counts as "wait, do not give up".
func TestManifestNotInstalledCountsAsUpdating(t *testing.T) {
	st := parseAppManifest(`"AppState" { "StateFlags" "2" "BytesToDownload" "1000" }`)
	if !st.Updating {
		t.Error("an update-required game should read as updating")
	}
}

// A finished download that Steam has not finished applying must keep us waiting
// rather than declaring the game ready and timing out.
func TestManifestDownloadedButNotAppliedKeepsWaiting(t *testing.T) {
	st := parseAppManifest(`"AppState" { "StateFlags" "4" "BytesToDownload" "1000" "BytesDownloaded" "400" }`)
	if !st.Updating {
		t.Error("a part-applied update should still count as updating")
	}
}

// Anything unreadable must report Known=false, so the caller keeps its ordinary
// behaviour instead of acting on a guess.
func TestUnreadableManifestIsNotGuessedAt(t *testing.T) {
	for _, bad := range []string{"", "not a vdf", "{}"} {
		if st := parseAppManifest(bad); st.Known {
			t.Errorf("parseAppManifest(%q) claimed to know something: %+v", bad, st)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:        "512 B",
		1536:       "1.5 KB",
		4294967296: "4.0 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
