//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"

	"queueup/internal/game"
	"queueup/internal/report"
)

// saveProblemReport writes one file to the Desktop containing the agent's log
// and the tail of the game's log. "Right-click the icon, save a report, send
// me the file" is the whole support flow: the agent knows where Rust's log is,
// so no player ever has to go looking for it.
func saveProblemReport() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("couldn't find your user folder")
	}
	desktop := filepath.Join(home, "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		// OneDrive sometimes relocates the Desktop; fall back to the home folder
		// rather than failing over furniture.
		desktop = home
	}
	return report.Build(desktop, report.Inputs{
		AgentVersion: Version,
		AgentLogPath: logFilePath(),
		GameLogPath:  game.DefaultLogPath(),
	})
}
