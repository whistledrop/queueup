//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// The agent starts at login through the per-user registry Run key, not a
// service and not Task Scheduler. Why: it needs no administrator rights, it
// runs in the user's session (which Steam and Rust need anyway), it survives
// Windows updates, and removing it is one registry value. A service would run
// before login, where launching a game is impossible anyway.

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runKeyName = "QueueUp"

func cmdInstallAutostart(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding this program's own path: %w", err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening the startup list: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(runKeyName, fmt.Sprintf(`"%s" tray`, exe)); err != nil {
		return fmt.Errorf("adding QueueUp to the startup list: %w", err)
	}
	fmt.Println("Done. QueueUp will start automatically when you sign in to Windows.")
	fmt.Println("Undo at any time with: agent uninstall-autostart")
	return nil
}

func cmdUninstallAutostart(args []string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening the startup list: %w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(runKeyName); err != nil {
		return fmt.Errorf("QueueUp wasn't in the startup list: %w", err)
	}
	fmt.Println("Done. QueueUp no longer starts automatically.")
	return nil
}
