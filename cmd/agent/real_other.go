//go:build !windows

package main

import (
	"errors"

	"queueup/internal/game"
)

// The real game only runs on Windows. On a Mac the agent is simulator-only,
// which is exactly how all development happens.
func realLauncher(string) (game.Launcher, error) {
	return nil, errors.New("the real game launcher only exists on Windows; use --sim on this machine")
}
