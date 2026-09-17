//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"queueup/internal/agentcfg"
	"queueup/internal/game"
	"queueup/internal/relayclient"
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

// sendProblemReport uploads the same report straight to QueueUp, so a beta
// tester never has to find a file and email it. If the upload fails for any
// reason, the report is saved to the Desktop instead and the returned path
// says where: a report that cannot be sent should still not be lost.
func sendProblemReport() (sent bool, savedTo string, err error) {
	in := report.Inputs{
		AgentVersion: Version,
		AgentLogPath: logFilePath(),
		GameLogPath:  game.DefaultLogPath(),
	}
	_, content := report.Render(in)

	cfgPath, cerr := agentcfg.DefaultPath()
	if cerr == nil {
		if cfg, lerr := agentcfg.Load(cfgPath); lerr == nil && cfg.Paired() {
			relay := cfg.RelayURL
			if relay == "" {
				relay = DefaultRelayURL
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if uerr := relayclient.SendReport(ctx, relay, cfg.DeviceToken, Version, content); uerr == nil {
				return true, "", nil
			} else {
				err = uerr
			}
		}
	}

	path, serr := saveProblemReport()
	if serr != nil {
		if err != nil {
			return false, "", fmt.Errorf("%v, and saving it failed too: %v", err, serr)
		}
		return false, "", serr
	}
	return false, path, err
}
