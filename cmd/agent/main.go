// Command agent is the QueueUp agent. It runs on the player's gaming PC.
//
//	agent pair --relay https://relay.example.com
//	    Link this PC to your account. Shows a code to type into the web app.
//
//	agent run --relay https://relay.example.com
//	    Stay connected to the relay and run joins on command. This is the mode
//	    that will run in the background with a tray icon.
//
//	agent sim --scenario testdata/scenarios/long_queue.json
//	    Run one join against the fake Rust client, with no relay involved.
//
//	agent status
//	    Show what this PC is set up with.
package main

import (
	"fmt"
	"os"
	"strings"
)

// Version is reported to the relay. Self-updates are out of scope for v1, but
// the relay records this so an "your agent is out of date" check can be added
// without changing the wire protocol.
const Version = "0.2.0-phase2"

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage())
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "pair":
		err = cmdPair(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "tray":
		err = cmdTray(os.Args[2:])
	case "install-autostart":
		err = cmdInstallAutostart(os.Args[2:])
	case "uninstall-autostart":
		err = cmdUninstallAutostart(os.Args[2:])
	case "sim":
		err = cmdSim(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Println(usage())
		return
	default:
		fmt.Println(usage())
		fmt.Fprintf(os.Stderr, "\nunknown command %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nerror:", err)
		os.Exit(1)
	}
}

func usage() string {
	return strings.TrimSpace(`
QueueUp agent ` + Version + `

  agent pair --relay <url>     link this PC to your account
  agent run  --relay <url>     stay connected and run joins on command
  agent tray                   the same, with a system tray icon (Windows)
  agent install-autostart      start QueueUp automatically at Windows login
  agent uninstall-autostart    stop doing that
  agent sim  --scenario <file> run one join against the fake Rust client
  agent status                 show what this PC is set up with

This tool never asks for, stores, or uses your Steam password.
`)
}
