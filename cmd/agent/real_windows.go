//go:build windows

package main

import "queueup/internal/game"

func realLauncher(logPath string) (game.Launcher, error) {
	return &game.WindowsLauncher{Log: logPath}, nil
}

// gameLogPath is where the agent would look for Rust's log on this machine.
func gameLogPath() string { return game.DefaultLogPath() }
