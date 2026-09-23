package main

import (
	"fmt"

	"queueup/internal/game"
)

// cmdSteamState prints exactly what QueueUp can see about Steam and Rust on
// this PC, and what it concludes from it.
//
// It exists because the force-wipe update path is the hardest thing here to be
// sure of: it only happens for real when Rust ships a patch, and the meaning of
// some of Steam's own status bits is inferred rather than documented. Running
// this while Steam is downloading, paused, or idle turns that inference into
// evidence, from a real machine, in one command.
func cmdSteamState(args []string) error {
	fmt.Println("What QueueUp can see about Steam and Rust on this PC")
	fmt.Println("---------------------------------------------------")

	path := game.RustManifestPath()
	if path == "" {
		fmt.Println("\nSteam's record of Rust: NOT FOUND.")
		fmt.Println("Either Rust is not installed on this PC, or Steam keeps its")
		fmt.Println("library somewhere QueueUp did not look. QueueUp falls back to its")
		fmt.Println("ordinary behaviour when this happens: it waits for the game as usual.")
		return nil
	}
	fmt.Printf("\nSteam's record of Rust:\n  %s\n", path)

	u := game.RustUpdateState()
	if !u.Known {
		fmt.Println("\n...but it could not be read just now. Try again in a moment.")
		return nil
	}

	fmt.Printf("\nRaw from Steam:\n")
	fmt.Printf("  StateFlags       %d\n", u.StateFlags)
	fmt.Printf("  bytes downloaded %d\n", u.BytesDownloaded)
	fmt.Printf("  bytes to go      %d\n", u.BytesToDownload)
	fmt.Printf("  install folder   %s\n", u.InstallDir)

	fmt.Printf("\nWhat QueueUp concludes:\n")
	fmt.Printf("  installed        %v\n", u.Installed)
	fmt.Printf("  updating         %v\n", u.Updating)
	fmt.Printf("  paused           %v\n", u.Paused)
	fmt.Printf("  needs you at PC  %v\n", u.NeedsPlayer())

	msg := u.Describe()
	if msg == "" {
		msg = "(nothing: QueueUp would say nothing about Steam right now)"
	}
	fmt.Printf("\nWhat your phone would say:\n  %s\n", msg)

	fmt.Printf("\nRust's log file, as QueueUp finds it:\n  %s\n", orNotFound(gameLogPath()))
	return nil
}

func orNotFound(s string) string {
	if s == "" {
		return "(not found: Rust has probably never run on this PC)"
	}
	return s
}
