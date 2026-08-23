package main

import (
	"path/filepath"

	"queueup/internal/agentcfg"
)

// logFilePath is where the agent writes its own log, next to the settings.
// The tray's "Open the log file" points here, and it is the file to send when
// reporting a problem.
func logFilePath() string {
	p, err := agentcfg.DefaultPath()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "agent.log")
}
