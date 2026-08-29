package game

import (
	"os"
	"path/filepath"
	"strings"
)

// Where is Rust's log?
//
// The honest answer, learned from a real PC on 2026-08-29: Rust launches itself
// with `-logfile output_log.txt`, so the log lives in the game's own install
// folder. The Unity default under AppData\LocalLow, which this project watched
// for its entire life until that day, is never written on a modern install.
// The agent was reading a file that did not exist, which made the whole game
// look silent: no queue positions, no join, and a deliberate quit
// indistinguishable from a crash.
//
// Both locations are still considered, install folder first, and when both
// somehow exist the freshest one wins, because the freshest one is the one the
// game is actually writing.

// FindRustLog picks the log path to watch. installDirs is where Rust is
// installed (found from Steam's records on Windows, empty elsewhere); home is
// the user's home folder for the legacy AppData location.
func FindRustLog(home string, installDirs []string) string {
	var candidates []string
	for _, dir := range installDirs {
		if dir != "" {
			candidates = append(candidates, filepath.Join(dir, "output_log.txt"))
		}
	}
	if home != "" {
		lowDir := filepath.Join(home, "AppData", "LocalLow")
		if entries, err := os.ReadDir(lowDir); err == nil {
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "facepunch") {
					candidates = append(candidates, filepath.Join(lowDir, e.Name(), "Rust", "Player.log"))
				}
			}
		}
	}

	// The freshest existing candidate is the one the game writes.
	best := ""
	var bestMod int64 = -1
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err != nil {
			continue
		}
		if mod := st.ModTime().UnixNano(); mod > bestMod {
			best, bestMod = c, mod
		}
	}
	if best != "" {
		return best
	}

	// Nothing exists yet: the game has not run since install. Watch where it
	// WILL write, which is the install folder when we know it.
	if len(candidates) > 0 {
		return candidates[0]
	}
	if home != "" {
		return filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust", "Player.log")
	}
	return ""
}
