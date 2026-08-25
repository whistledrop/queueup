package game

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
	Updating bool
	// Paused is true when Steam has an update to do but has stopped doing it.
	// A paused download will never finish on its own, so this needs the player,
	// not patience.
	Paused          bool
	BytesDownloaded int64
	BytesToDownload int64
	// StalledFor is how long the byte count has sat still. Steam does not
	// always mark a wedged download as paused (a full disk, or a Steam that
	// lost its connection, often just stop), so the only honest signal is that
	// nothing is moving.
	StalledFor time.Duration
}

// StallReport is how long a download must sit still before we stop believing it
// is going to finish. Long enough to ride out a slow patch server or a pause
// while Steam verifies, short enough to be useful on wipe day.
const StallReport = 8 * time.Minute

// NeedsPlayer is true when waiting will not fix this and the player has to go
// and look at the PC.
func (u UpdateState) NeedsPlayer() bool {
	return u.Updating && (u.Paused || u.StalledFor >= StallReport)
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
//
// A download that is moving and one that has stopped need different sentences.
// The first asks for patience. The second has to say plainly that nothing is
// happening, because otherwise the phone reassures the player right up until
// they miss the wipe.
func (u UpdateState) Describe() string {
	switch {
	case !u.Updating:
		return ""

	case u.Paused:
		return fmt.Sprintf("Steam has paused the Rust download%s. It will not finish on its own: open Steam on your PC and resume it.",
			u.progressSuffix())

	case u.StalledFor >= StallReport:
		return fmt.Sprintf("Steam has stopped downloading Rust%s. Nothing has moved for %d minutes. Check Steam on your PC: it may be offline, out of disk space, or paused.",
			u.progressSuffix(), int(u.StalledFor.Minutes()))

	case u.BytesToDownload > 0:
		return fmt.Sprintf("Steam is updating Rust, %d%% of %s downloaded. This happens on force wipe. Your PC will connect as soon as it finishes.",
			u.Percent(), humanBytes(u.BytesToDownload))

	default:
		return "Steam is updating Rust. This happens on force wipe. Your PC will connect as soon as it finishes."
	}
}

// progressSuffix adds "(1.0 GB of 4.0 GB done)" when we know the numbers.
func (u UpdateState) progressSuffix() string {
	if u.BytesToDownload <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%s of %s done)", humanBytes(u.BytesDownloaded), humanBytes(u.BytesToDownload))
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
	st.Paused = st.Updating && flags&stateUpdatePaused != 0
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

// stallWatch notices when a download stops moving.
//
// Steam's manifest says what Steam intends, not whether it is getting anywhere.
// A Steam that has gone offline, or run out of disk, often keeps claiming an
// update is in progress while the byte count sits still. Watching the bytes is
// the only way to tell the difference between slow and stuck.
type stallWatch struct {
	now func() time.Time

	mu        sync.Mutex
	active    bool
	lastBytes int64
	since     time.Time
}

func newStallWatch(now func() time.Time) *stallWatch {
	if now == nil {
		now = time.Now
	}
	return &stallWatch{now: now}
}

// stalledFor records the latest reading and returns how long the download has
// been sitting still. It is zero while an update is progressing, and zero when
// no update is happening at all.
func (s *stallWatch) stalledFor(u UpdateState) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !u.Known || !u.Updating {
		s.active = false
		return 0
	}
	now := s.now()

	// First sighting of this update, or the download moved: start the clock
	// again from here.
	if !s.active || u.BytesDownloaded != s.lastBytes {
		s.active = true
		s.lastBytes = u.BytesDownloaded
		s.since = now
		return 0
	}
	return now.Sub(s.since)
}
