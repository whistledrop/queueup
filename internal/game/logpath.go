package game

import (
	"os"
	"path/filepath"
	"strings"
)

// FindRustLog locates Rust's log file under a given home directory.
//
// Unity writes to AppData\\LocalLow\\<company>\\<product>, and Rust's company
// folder is "Facepunch Studios LTD", the studio's legal name. That was guessed
// as plain "Facepunch" until a real PC proved otherwise, so this does not hard
// code either: it takes whatever Facepunch-ish folder actually exists.
//
// When nothing exists yet, which is normal before the game has ever run, it
// returns the known name so the tailer has somewhere to watch.
func FindRustLog(home string) string {
	if home == "" {
		return ""
	}
	lowDir := filepath.Join(home, "AppData", "LocalLow")

	if entries, err := os.ReadDir(lowDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(strings.ToLower(e.Name()), "facepunch") {
				continue
			}
			candidate := filepath.Join(lowDir, e.Name(), "Rust", "Player.log")
			if _, err := os.Stat(filepath.Dir(candidate)); err == nil {
				return candidate
			}
		}
	}
	return filepath.Join(lowDir, "Facepunch Studios LTD", "Rust", "Player.log")
}
