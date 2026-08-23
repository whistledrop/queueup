//go:build windows

package main

// The agent is built with -H windowsgui so that starting the tray at login
// never flashes a console window. The cost: a windowsgui program has no
// standard output, so `agent pair` run from a terminal would print nothing.
// This file gives the output back: when started from a terminal, reattach to
// that terminal's console before doing anything else.

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
)

const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS

func init() {
	// Use the console of whoever launched us, if they have one. Launched from
	// the Start menu or the Run key there is none, the call fails, and that is
	// exactly right: no window appears.
	r, _, _ := procAttachConsole.Call(attachParentProcess)
	if r == 0 {
		return
	}
	for _, f := range []struct {
		name string
		mode int
		std  **os.File
	}{
		{"CONOUT$", os.O_WRONLY, &os.Stdout},
		{"CONOUT$", os.O_WRONLY, &os.Stderr},
		{"CONIN$", os.O_RDONLY, &os.Stdin},
	} {
		h, err := os.OpenFile(f.name, f.mode, 0)
		if err == nil {
			*f.std = h
		}
	}
}
