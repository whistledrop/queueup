package game

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Force wipe is the first Thursday of the month, and it is the day this whole
// product exists for. It is also the day Rust ships a client update: the server
// restarts, and every player must download several gigabytes through Steam
// before the game will start at all.
//
// So on the most important day, "the game did not appear" is the NORMAL state
// for a long while, and treating it as a crash would make QueueUp fail exactly
// when it matters. This file reads Steam's own record of what it is doing, so
// the agent can wait patiently and tell the player what is happening.
//
// Only Steam's app manifest is read, never a game file, and nothing is written.

// UpdateState is what Steam is doing with Rust right now.
type UpdateState struct {
	// Known is false when we could not find Steam's manifest at all, in which
	// case the caller should carry on with its ordinary behaviour rather than
	// assume anything.
	Known bool
	// Installed is true when the game is ready to launch.
	Installed bool
	// Updating is true when Steam is downloading or applying an update.
	Updating        bool
	BytesDownloaded int64
	BytesToDownload int64
}

// Percent is how far through the download Steam is, 0 when unknown.
func (u UpdateState) Percent() int {
	if u.BytesToDownload <= 0 {
		return 0
	}
	p := int(float64(u.BytesDownloaded) / float64(u.BytesToDownload) * 100)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// Describe is the sentence shown on the player's phone.
func (u UpdateState) Describe() string {
	switch {
	case !u.Updating:
		return ""
	case u.BytesToDownload > 0:
		return fmt.Sprintf("Steam is updating Rust, %d%% of %s downloaded. This happens on force wipe. Your PC will connect as soon as it finishes.",
			u.Percent(), humanBytes(u.BytesToDownload))
	default:
		return "Steam is updating Rust. This happens on force wipe. Your PC will connect as soon as it finishes."
	}
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Steam's manifest is a VDF file: quoted key then quoted value, one per line.
var vdfPair = regexp.MustCompile(`"([^"]+)"\s+"([^"]*)"`)

// parseAppManifest reads the fields we care about out of an appmanifest .acf.
//
// StateFlags is a bitmask. Bit 4 means fully installed; the update bits mean
// Steam still has work to do before the game can run.
func parseAppManifest(content string) UpdateState {
	const (
		stateUpdateRequired = 1 << 1
		stateFullyInstalled = 1 << 2
		stateUpdateRunning  = 1 << 8
		stateUpdatePaused   = 1 << 9
		stateUpdateStarted  = 1 << 10
	)

	fields := map[string]string{}
	for _, m := range vdfPair.FindAllStringSubmatch(content, -1) {
		fields[strings.ToLower(m[1])] = m[2]
	}
	if len(fields) == 0 {
		return UpdateState{}
	}

	num := func(key string) int64 {
		v, _ := strconv.ParseInt(fields[key], 10, 64)
		return v
	}
	flags := num("stateflags")

	st := UpdateState{
		Known:           true,
		Installed:       flags&stateFullyInstalled != 0,
		BytesDownloaded: num("bytesdownloaded"),
		BytesToDownload: num("bytestodownload"),
	}
	st.Updating = flags&(stateUpdateRequired|stateUpdateRunning|stateUpdatePaused|stateUpdateStarted) != 0 ||
		!st.Installed
	// A finished download that has not been marked installed yet still counts as
	// updating, which is what we want: keep waiting.
	if st.Installed && st.BytesToDownload > 0 && st.BytesDownloaded < st.BytesToDownload {
		st.Updating = true
	}
	return st
}

func readAppManifest(path string) (UpdateState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return UpdateState{}, err
	}
	return parseAppManifest(string(raw)), nil
}
