//go:build !windows

package main

import "errors"

// The tray, like the game, is Windows-only. Development machines use `run`.
func cmdTray([]string) error {
	return errors.New("the tray icon only exists on Windows; use: agent run")
}

func cmdInstallAutostart([]string) error {
	return errors.New("autostart setup only exists on Windows")
}

func cmdUninstallAutostart([]string) error {
	return errors.New("autostart setup only exists on Windows")
}
