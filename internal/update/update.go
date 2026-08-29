// Package update keeps the agent current by itself.
//
// The lesson that forced this: four hand-updates in one afternoon, each one
// needing a person at the PC to quit the tray, download, replace and restart.
// With customers, every fix shipped would strand people on broken versions,
// and the people it strands are the ones mid-wipe.
//
// The mechanics are deliberately boring. The agent asks GitHub what the newest
// release is, downloads the one file, and swaps itself using the one trick
// Windows allows: a running exe cannot be overwritten, but it CAN be renamed.
// So the running copy renames itself out of the way, puts the new file in its
// place, starts it, and exits. The new copy deletes the leftover on startup.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Release is what a check found.
type Release struct {
	Version  string // the tag, e.g. "v0.2.0"
	AssetURL string // direct download for the agent exe
}

// releasesURL is asked for the newest release. Unauthenticated: GitHub allows
// plenty of anonymous requests for our once-every-few-hours check.
// releasesURLForTest lets tests point the check at a fake server.
var releasesURLForTest = "https://api.github.com/repos/whistledrop/queueup/releases/latest"

const assetName = "QueueUpAgent.exe"

// Check asks what the newest release is. It does not decide anything; pair it
// with ShouldUpdate.
func Check(client *http.Client) (Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, releasesURLForTest, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("couldn't reach the update server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("the update server answered %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("couldn't read the update answer: %w", err)
	}
	rel := Release{Version: body.TagName}
	for _, a := range body.Assets {
		if a.Name == assetName {
			rel.AssetURL = a.URL
		}
	}
	if rel.Version == "" || rel.AssetURL == "" {
		return Release{}, fmt.Errorf("the newest release is missing its download")
	}
	return rel, nil
}

// ShouldUpdate says whether moving from current to latest is right.
//
// It refuses two things on purpose. A development build (anything that is not
// a clean vX.Y.Z) is somebody's work in progress and must never be clobbered.
// And a DOWNGRADE is refused, so a stale cache or a rolled-back release page
// cannot push an agent backwards.
func ShouldUpdate(current, latest string) bool {
	cur, okCur := parseVersion(current)
	lat, okLat := parseVersion(latest)
	if !okCur || !okLat {
		return false
	}
	for i := 0; i < 3; i++ {
		if lat[i] != cur[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

// parseVersion reads a clean "vX.Y.Z" and nothing else.
func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// Download fetches the release into dir and sanity-checks that what arrived is
// a Windows program of plausible size, not an error page.
func Download(client *http.Client, rel Release, dir string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Get(rel.AssetURL)
	if err != nil {
		return "", fmt.Errorf("downloading the update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the update download answered %s", resp.Status)
	}

	path := filepath.Join(dir, assetName+".next")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("saving the update: %w", err)
	}
	if err := looksLikeAWindowsProgram(path, n); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// looksLikeAWindowsProgram is the sanity check between "downloaded something"
// and "replaced the agent with it".
func looksLikeAWindowsProgram(path string, size int64) error {
	const minPlausible = 1 << 20 // the real agent is several megabytes
	if size < minPlausible {
		return fmt.Errorf("the downloaded update is implausibly small (%d bytes)", size)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 2)
	if _, err := io.ReadFull(f, head); err != nil {
		return err
	}
	if head[0] != 'M' || head[1] != 'Z' {
		return fmt.Errorf("the downloaded update is not a Windows program")
	}
	return nil
}

// oldSuffix marks the renamed-away previous version.
const oldSuffix = ".old.exe"

// Apply swaps newFile into exePath's place. The running exe is renamed aside,
// which Windows permits, and the new file moved in. On any failure it puts
// things back, so a half-applied update cannot leave the PC with no agent.
func Apply(exePath, newFile string) error {
	old := exePath + oldSuffix
	// A leftover from last time would block the rename.
	_ = os.Remove(old)

	if err := os.Rename(exePath, old); err != nil {
		return fmt.Errorf("moving the current version aside: %w", err)
	}
	if err := os.Rename(newFile, exePath); err != nil {
		// Put the world back exactly as it was.
		if undo := os.Rename(old, exePath); undo != nil {
			return fmt.Errorf("installing the update failed (%v) AND restoring failed (%v): reinstall from the website", err, undo)
		}
		return fmt.Errorf("installing the update: %w", err)
	}
	return nil
}

// CleanupOld removes the renamed-away previous version. Called by the NEW copy
// on startup; if the old process has not quite exited yet the file is briefly
// locked, so a failure here is fine and retried on the next start.
func CleanupOld(exePath string) {
	_ = os.Remove(exePath + oldSuffix)
}
