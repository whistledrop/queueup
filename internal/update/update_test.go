package update

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldUpdate(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
		why             string
	}{
		{"v0.1.9", "v0.1.10", true, "ordinary upgrade"},
		{"v0.1.10", "v0.2.0", true, "minor bump"},
		{"v0.9.9", "v1.0.0", true, "major bump"},
		{"v0.1.10", "v0.1.10", false, "already current"},
		{"v0.2.0", "v0.1.10", false, "a downgrade must be refused"},
		{"v0.1.10", "v0.1.9", false, "a rolled-back release page must not push agents backwards"},
		{"dev", "v0.2.0", false, "a development build is someone's work in progress"},
		{"v0.1.6-4-g0bb436f", "v0.2.0", false, "a between-releases build is not auto-updated"},
		{"v0.1.9", "not-a-version", false, "garbage from the server changes nothing"},
	}
	for _, c := range cases {
		if got := ShouldUpdate(c.current, c.latest); got != c.want {
			t.Errorf("ShouldUpdate(%q, %q) = %v, want %v (%s)", c.current, c.latest, got, c.want, c.why)
		}
	}
}

func TestCheckReadsARelease(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"assets": [
				{"name": "checksums.txt", "browser_download_url": "https://example.com/sums"},
				{"name": "QueueUpAgent.exe", "browser_download_url": "https://example.com/agent.exe"}
			]
		}`))
	}))
	defer ts.Close()

	old := releasesURLForTest
	releasesURLForTest = ts.URL
	defer func() { releasesURLForTest = old }()

	rel, err := Check(ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "v0.2.0" || rel.AssetURL != "https://example.com/agent.exe" {
		t.Fatalf("Check = %+v", rel)
	}
}

func TestCheckRefusesAReleaseWithNoAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v0.2.0", "assets": []}`))
	}))
	defer ts.Close()
	old := releasesURLForTest
	releasesURLForTest = ts.URL
	defer func() { releasesURLForTest = old }()

	if _, err := Check(ts.Client()); err == nil {
		t.Fatal("a release with no exe was accepted")
	}
}

func exeBytes(size int) []byte {
	b := bytes.Repeat([]byte{0}, size)
	b[0], b[1] = 'M', 'Z'
	return b
}

func TestDownloadSanityChecks(t *testing.T) {
	payload := exeBytes(2 << 20)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			_, _ = w.Write(payload)
		case "/tiny":
			_, _ = w.Write([]byte("MZ tiny"))
		case "/html":
			_, _ = w.Write(bytes.Repeat([]byte("<html>error page</html>"), 100000))
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	if _, err := Download(ts.Client(), Release{AssetURL: ts.URL + "/good"}, dir); err != nil {
		t.Fatalf("a plausible exe was refused: %v", err)
	}
	if _, err := Download(ts.Client(), Release{AssetURL: ts.URL + "/tiny"}, dir); err == nil {
		t.Fatal("a 7-byte 'update' was accepted; the agent would have replaced itself with it")
	}
	if _, err := Download(ts.Client(), Release{AssetURL: ts.URL + "/html"}, dir); err == nil {
		t.Fatal("an HTML error page was accepted as a Windows program")
	}
}

// The swap itself: current renamed aside, new moved in, leftovers cleaned.
func TestApplySwapsAndCleanupRemovesTheLeftover(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "QueueUpAgent.exe")
	if err := os.WriteFile(exe, []byte("old version"), 0o755); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(dir, "QueueUpAgent.exe.next")
	if err := os.WriteFile(next, []byte("new version"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Apply(exe, next); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "new version" {
		t.Fatalf("exe content = %q, %v; want the new version in place", got, err)
	}
	if _, err := os.Stat(exe + oldSuffix); err != nil {
		t.Fatal("the old version was not kept aside during the swap")
	}

	CleanupOld(exe)
	if _, err := os.Stat(exe + oldSuffix); !os.IsNotExist(err) {
		t.Fatal("the leftover old version was not cleaned up")
	}
}

// A failed swap must put the original back: a half-applied update that leaves
// the PC with NO agent is the one truly unacceptable outcome.
func TestAFailedApplyRestoresTheOriginal(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "QueueUpAgent.exe")
	if err := os.WriteFile(exe, []byte("old version"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The new file does not exist, so the second rename must fail.
	err := Apply(exe, filepath.Join(dir, "does-not-exist"))
	if err == nil {
		t.Fatal("applying a missing file succeeded")
	}
	got, rerr := os.ReadFile(exe)
	if rerr != nil || string(got) != "old version" {
		t.Fatalf("after a failed update the agent is %q, %v; want the original restored", got, rerr)
	}
}
