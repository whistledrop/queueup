//go:build !windows

package main

// On a Mac or Linux the terminal is already there; nothing to arrange.
func ensureConsole() {}
func waitForReader() {}
