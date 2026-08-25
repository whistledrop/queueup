//go:build !windows

package main

import "errors"

// trayAvailable says whether this build has a system tray. The tray, like the
// game itself, is Windows-only; development machines use `agent run`.
const trayAvailable = false

func cmdTray([]string) error {
	return errors.New("the tray icon only exists on Windows; use: agent run")
}

func cmdInstallAutostart([]string) error {
	return errors.New("autostart setup only exists on Windows")
}

func cmdUninstallAutostart([]string) error {
	return errors.New("autostart setup only exists on Windows")
}
