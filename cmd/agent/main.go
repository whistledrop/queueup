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

	"queueup/internal/agentcfg"
)

// Version is reported to the relay and stamped at build time by
// scripts/build-agent.sh. Self-updates are out of scope for v1, but the relay
// records this so an "your agent is out of date" check can be added without
// changing the wire protocol.
var Version = "dev"

// DefaultRelayURL and DefaultWebURL are baked in at build time by
// scripts/build-agent.sh, so the downloaded exe knows where its own service
// lives. Without them the user would have to type a URL, which is the kind of
// step that loses people.
var (
	DefaultRelayURL = ""
	DefaultWebURL   = ""
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		// Double-clicked. Do the obvious thing rather than printing usage at a
		// window that is not there: pair if this PC has never been linked,
		// otherwise start up and sit in the tray.
		args = []string{defaultCommand()}
	}
	if args[0] != "tray" {
		// These commands talk to the person, so make sure there is somewhere for
		// the words to go.
		ensureConsole()
	}

	var err error
	switch args[0] {
	case "pair":
		err = cmdPair(args[1:])
	case "run":
		err = cmdRun(args[1:])
	case "tray":
		err = cmdTray(args[1:])
	case "install-autostart":
		err = cmdInstallAutostart(args[1:])
	case "uninstall-autostart":
		err = cmdUninstallAutostart(args[1:])
	case "sim":
		err = cmdSim(args[1:])
	case "status":
		err = cmdStatus(args[1:])
	case "-h", "--help", "help":
		fmt.Println(usage())
		return
	default:
		fmt.Println(usage())
		fmt.Fprintf(os.Stderr, "\nunknown command %q\n", args[0])
		waitForReader()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nerror:", err)
		waitForReader()
		os.Exit(1)
	}
}

// defaultCommand is what a double-click means: link this PC if it has never
// been linked, otherwise get on with the job.
func defaultCommand() string {
	path, err := agentcfg.DefaultPath()
	if err != nil {
		return "pair"
	}
	cfg, err := agentcfg.Load(path)
	if err != nil || !cfg.Paired() {
		return "pair"
	}
	return "tray"
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
