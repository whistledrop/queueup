//go:build windows

package main

import "queueup/internal/game"

func realLauncher(logPath string) (game.Launcher, error) {
	return &game.WindowsLauncher{Log: logPath}, nil
}
