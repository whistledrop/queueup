//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"

	"queueup/internal/agentcfg"
)

// The agent starts at login through the per-user registry Run key, not a
// service and not Task Scheduler. Why: it needs no administrator rights, it
// runs in the user's session (which Steam and Rust need anyway), it survives
// Windows updates, and removing it is one registry value. A service would run
// before login, where launching a game is impossible anyway.

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runKeyName = "QueueUp"

// ensureAutostart makes "QueueUp comes back after a restart" true by default.
//
// It used to be opt-in, behind a tray checkbox or a typed command, and that was
// wrong. The promise of this product is that the PC waits at home on its own;
// somebody who pairs their PC, sees it work, and never finds the checkbox has a
// setup that silently ends at the next Windows update. They find out on wipe
// day, which is the worst possible day to find out.
//
// A deliberate "no" is still respected: unticking the box records the choice,
// and this leaves it alone from then on.
//
// It also refreshes the stored path. People move the exe after installing it,
// and a startup entry pointing at a file that is no longer there fails silently
// at exactly the moment nobody is watching.
func ensureAutostart() {
	path, err := resolveConfigPath("")
	if err != nil {
		return
	}
	cfg, err := agentcfg.Load(path)
	if err != nil || cfg.AutostartOff {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	want := fmt.Sprintf(`"%s" tray`, exe)

	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	if have, _, err := key.GetStringValue(runKeyName); err == nil && have == want {
		return
	}
	_ = key.SetStringValue(runKeyName, want)
}

// autostartInstalled reports whether QueueUp is in the startup list, so the
// tray can show the setting's real state rather than a guess.
func autostartInstalled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	v, _, err := key.GetStringValue(runKeyName)
	return err == nil && v != ""
}

// setAutostart is the tray's version of the two commands below: no terminal,
// no typing. Most people who install this will never open a command prompt,
// and should not have to.
//
// Turning it off is remembered, so that pairing again does not silently switch
// it back on.
func setAutostart(on bool) error {
	if path, err := resolveConfigPath(""); err == nil {
		if cfg, err := agentcfg.Load(path); err == nil {
			cfg.AutostartOff = !on
			_ = agentcfg.Save(path, cfg)
		}
	}
	if on {
		return cmdInstallAutostart(nil)
	}
	return cmdUninstallAutostart(nil)
}

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
