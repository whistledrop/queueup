//go:build !windows

package main

// Starting with the operating system is a Windows idea here, because the game
// is. On anything else these are no-ops so the shared code can call them
// without caring which machine it is on.
func autostartInstalled() bool { return false }

func ensureAutostart() {}
