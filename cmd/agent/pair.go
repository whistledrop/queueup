package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"queueup/internal/agentcfg"
	"queueup/internal/relayclient"
)

func cmdPair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	relayURL := fs.String("relay", "", "the QueueUp relay address, e.g. https://relay.example.com")
	webURL := fs.String("web", "", "the QueueUp website address, for the tray menu (optional)")
	configPath := fs.String("config", "", "settings file (default: your user config folder)")
	name := fs.String("name", "", "a name for this PC, shown in the web app")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path, err := resolveConfigPath(*configPath)
	if err != nil {
		return err
	}
	cfg, err := agentcfg.Load(path)
	if err != nil {
		return err
	}
	if *relayURL != "" {
		cfg.RelayURL = *relayURL
	}
	if cfg.RelayURL == "" {
		return fmt.Errorf("which relay should this PC connect to? Pass --relay")
	}
	if *webURL != "" {
		cfg.WebURL = *webURL
	}
	if *name != "" {
		cfg.DeviceName = *name
	}
	if cfg.DeviceName == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = "My gaming PC"
		}
		cfg.DeviceName = host
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start, err := relayclient.StartPairing(ctx, cfg.RelayURL, cfg.DeviceName)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Type this code into the QueueUp web app:")
	fmt.Println()
	fmt.Printf("        %s\n", spaced(start.Code))
	fmt.Println()
	fmt.Printf("  It expires in %s. Waiting...\n\n", time.Until(start.ExpiresAt).Round(time.Minute))

	token, err := relayclient.WaitForPairing(ctx, cfg.RelayURL, start.ClaimToken, 2*time.Second)
	if err != nil {
		return err
	}

	cfg.DeviceToken = token
	cfg.DeviceID = start.DeviceID
	if err := agentcfg.Save(path, cfg); err != nil {
		return err
	}

	fmt.Println("  This PC is now linked to your account.")
	fmt.Printf("  Settings saved to %s\n\n", path)
	fmt.Println("  Next: leave the agent running with")
	fmt.Printf("    agent run --relay %s\n", cfg.RelayURL)
	return nil
}

// spaced makes a code easier to read off a screen and type on a phone.
func spaced(code string) string {
	return strings.Join(strings.Split(code, ""), " ")
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "settings file (default: your user config folder)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := resolveConfigPath(*configPath)
	if err != nil {
		return err
	}
	cfg, err := agentcfg.Load(path)
	if err != nil {
		return err
	}
	fmt.Printf("QueueUp agent %s\n\n", Version)
	fmt.Printf("  settings file: %s\n", path)
	fmt.Printf("  relay:         %s\n", orNone(cfg.RelayURL))
	fmt.Printf("  this PC:       %s\n", orNone(cfg.DeviceName))
	if cfg.Paired() {
		fmt.Printf("  paired:        yes (device %s)\n", cfg.DeviceID)
	} else {
		fmt.Printf("  paired:        no. Run: agent pair --relay <url>\n")
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func resolveConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return agentcfg.DefaultPath()
}
