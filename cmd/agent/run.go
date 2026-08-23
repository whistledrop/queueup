package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"queueup/internal/agentapp"
	"queueup/internal/agentcfg"
	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/protocol"
	"queueup/internal/relayclient"
	"queueup/internal/scenario"
)

// statusSink, when set (by the tray), receives one-line status text.
var statusSink func(string)

// cmdRun is the mode the agent lives in on a player's PC: connected to the
// relay, waiting for something to do. In phase 2 it runs in a terminal; the tray
// icon wraps exactly this in a later phase.
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var (
		relayURL     = fs.String("relay", "", "the QueueUp relay address (defaults to the one saved at pairing)")
		configPath   = fs.String("config", "", "settings file (default: your user config folder)")
		patternsPath = fs.String("patterns", "configs/patterns.json", "log pattern file")
		useSim       = fs.Bool("sim", false, "run against the fake Rust client instead of the real game")
		scenarioPath = fs.String("scenario", "", "which scenario the fake Rust client should play (with --sim)")
		speed        = fs.Float64("speed", 1, "simulator timeline speed multiplier")
		logPath      = fs.String("log", "", "Rust log file to watch (default: the real Windows location, or a temp file with --sim)")
		confirm      = fs.Duration("confirm", 30*time.Second, "how long to hold a slot before calling the job done")
		maxAttempts  = fs.Int("max-attempts", 8, "how many times to try before giving up")
		verbose      = fs.Bool("verbose", false, "send raw Rust log lines to the relay's debug view")
	)
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
		return errors.New("this PC has no relay set. Run: agent pair --relay <url>")
	}
	if !cfg.Paired() {
		return fmt.Errorf("this PC isn't paired yet. Run: agent pair --relay %s", cfg.RelayURL)
	}

	parser, err := logparse.Load(*patternsPath)
	if err != nil {
		return err
	}

	// The log goes to the console AND to a file next to the settings, so the
	// tray's "Open the log file" always has something to show and a problem on
	// a headless PC can still be diagnosed afterwards.
	logOut := io.Writer(os.Stdout)
	if lf := logFilePath(); lf != "" {
		if err := os.MkdirAll(filepath.Dir(lf), 0o700); err == nil {
			if f, ferr := os.OpenFile(lf, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); ferr == nil {
				defer f.Close()
				logOut = io.MultiWriter(os.Stdout, f)
			}
		}
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelInfo}))

	newGame, err := launcherFactory(*useSim, *scenarioPath, *logPath, *speed)
	if err != nil {
		return err
	}

	host, _ := os.Hostname()
	client := &relayclient.Client{
		RelayURL:     cfg.RelayURL,
		DeviceToken:  cfg.DeviceToken,
		AgentVersion: Version,
		OS:           runtime.GOOS,
		Hostname:     host,
		Simulator:    *useSim,
		Log:          log,
	}
	app := &agentapp.App{
		Client:       client,
		Parser:       parser,
		NewGame:      newGame,
		Log:          log,
		SendLogLines: *verbose,
		OnStatusText: statusSink,
		JobConfig: job.Config{
			MaxAttempts:     *maxAttempts,
			InServerConfirm: *confirm,
		},
	}
	client.Handler = app

	fmt.Printf("QueueUp agent %s\n", Version)
	fmt.Printf("relay: %s\n", cfg.RelayURL)
	if *useSim {
		fmt.Printf("mode:  SIMULATOR (%s)\n", *scenarioPath)
	} else {
		fmt.Printf("mode:  REAL GAME\n")
	}
	if un := parser.Unverified(); len(un) > 0 {
		fmt.Printf("\n  WARNING: %d log patterns are still guesses, not yet confirmed against a\n"+
			"  real Player.log. Fix by editing %s.\n", len(un), *patternsPath)
	}
	fmt.Println("\nwaiting for jobs. Press Ctrl-C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = client.Run(ctx)

	// Stop the running job without reporting it finished. The relay still has
	// it, and hands it straight back when this agent starts up again. That is
	// the same path a surprise Windows reboot takes.
	app.Stop()

	if err != nil {
		return err
	}
	fmt.Println("stopped.")
	return nil
}

// launcherFactory decides what starts the game: the fake Rust client, or the
// real one on Windows.
func launcherFactory(useSim bool, scenarioPath, logPath string, speed float64) (agentapp.LauncherFactory, error) {
	if !useSim {
		return func(protocol.Job) (game.Launcher, error) {
			return realLauncher(logPath)
		}, nil
	}
	if scenarioPath == "" {
		return nil, errors.New("--sim needs a --scenario file")
	}
	sc, err := scenario.Load(scenarioPath)
	if err != nil {
		return nil, err
	}
	return func(j protocol.Job) (game.Launcher, error) {
		p := logPath
		if p == "" {
			// One log file per job, so a previous job's lines can never be
			// mistaken for this one's.
			p = filepath.Join(os.TempDir(), "queueup-"+j.ID+"-Player.log")
		}
		return &game.SimLauncher{Scenario: sc, Log: p, Speed: speed}, nil
	}, nil
}
