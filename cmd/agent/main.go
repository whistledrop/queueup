// Command agent is the QueueUp agent.
//
// Phase 1: it runs one join job on this machine and prints what it is doing to
// the console. There is no relay connection and no tray icon yet; those arrive
// in phase 2. Everything below the command-line layer is the code that will
// ship on the player's PC.
//
// Try it:
//
//	go run ./cmd/agent --sim --scenario testdata/scenarios/long_queue.json --server 1.2.3.4:28015
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/runner"
	"queueup/internal/scenario"
	"queueup/internal/serverstat"
)

// Version is stamped at build time later; kept here so update checks can be
// added in a future phase without restructuring anything.
const Version = "0.1.0-phase1"

func main() {
	var (
		sim          = flag.Bool("sim", false, "use the fake Rust simulator instead of the real game")
		scenarioPath = flag.String("scenario", "", "scenario file to simulate (requires --sim)")
		serverAddr   = flag.String("server", "127.0.0.1:28015", "target server as IP:PORT")
		patternsPath = flag.String("patterns", "configs/patterns.json", "log pattern file")
		logPath      = flag.String("log", "", "Rust log file to watch (default: the real Windows location, or a temp file in --sim)")
		waitForUp    = flag.Bool("wait-for-server-up", false, "wait for the server to come back after a wipe restart before connecting")
		speed        = flag.Float64("speed", 1, "simulator timeline speed multiplier")
		jitter       = flag.Duration("jitter", time.Second, "maximum random delay before connecting after the server comes up")
		maxAttempts  = flag.Int("max-attempts", 8, "how many times to try before giving up")
		confirm      = flag.Duration("confirm", 5*time.Second, "how long to hold a slot before calling the job done")
		showAllLines = flag.Bool("verbose", false, "print every log line, not just the ones we understand")
	)
	flag.Parse()

	if err := run(*sim, *scenarioPath, *serverAddr, *patternsPath, *logPath, *waitForUp,
		*speed, *jitter, *maxAttempts, *confirm, *showAllLines); err != nil {
		fmt.Fprintln(os.Stderr, "\nerror:", err)
		os.Exit(1)
	}
}

func run(sim bool, scenarioPath, serverAddr, patternsPath, logPath string, waitForUp bool,
	speed float64, jitter time.Duration, maxAttempts int, confirm time.Duration, verbose bool) error {

	addr, err := game.ParseAddr(serverAddr)
	if err != nil {
		return err
	}

	parser, err := logparse.Load(patternsPath)
	if err != nil {
		return err
	}
	fmt.Printf("QueueUp agent %s\n", Version)
	fmt.Printf("target server: %s\n", addr)
	if un := parser.Unverified(); len(un) > 0 {
		fmt.Printf("\n  WARNING: %d of the log patterns are still guesses, not confirmed against a\n"+
			"  real Player.log: %s\n"+
			"  Until they are verified the agent may misread the real game.\n"+
			"  Fix by editing %s. No rebuild needed.\n\n",
			len(un), strings.Join(un, ", "), patternsPath)
	}

	var launcher game.Launcher
	var server serverstat.Source = serverstat.AlwaysUp{}

	if sim {
		if scenarioPath == "" {
			return fmt.Errorf("--sim needs a --scenario file")
		}
		sc, err := scenario.Load(scenarioPath)
		if err != nil {
			return err
		}
		if logPath == "" {
			logPath = filepath.Join(os.TempDir(), "queueup-fake-Player.log")
		}
		fmt.Printf("mode: SIMULATOR, scenario %q\n", sc.Name)
		fmt.Printf("  %s\n", sc.Description)
		fmt.Printf("fake log: %s\n\n", logPath)

		launcher = &game.SimLauncher{Scenario: sc, Log: logPath, Speed: speed}
		if len(sc.Server) > 0 {
			server = serverstat.NewScripted(sc.Server, speed)
			waitForUp = true
		}
	} else {
		l, err := realLauncher(logPath)
		if err != nil {
			return err
		}
		launcher = l
		fmt.Printf("mode: REAL GAME\nlog: %s\n\n", launcher.LogPath())
	}

	m := job.New(job.Config{
		WaitForServerUp:  waitForUp,
		MaxAttempts:      maxAttempts,
		ConnectJitterMax: jitter,
		InServerConfirm:  confirm,
		RetryBase:        2 * time.Second,
		RetryMax:         20 * time.Second,
	})

	start := time.Now()
	r := &runner.Runner{
		Machine:  m,
		Launcher: launcher,
		Parser:   parser,
		Server:   server,
		Addr:     addr,
		OnTransition: func(t job.Transition) {
			fmt.Printf("[%6.1fs] %-22s %s\n", time.Since(start).Seconds(), string(t.To), t.Detail)
		},
		OnLogLine: func(line string, understood bool) {
			if verbose {
				mark := " "
				if understood {
					mark = "*"
				}
				fmt.Printf("           %s log: %s\n", mark, line)
			}
		},
	}

	// Ctrl-C behaves like the phone's cancel button: cancel the job cleanly and
	// close the game, rather than leaving Rust sitting in a queue.
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		fmt.Println("\ncancelling...")
		r.Cancel("You cancelled the join.")
		cancelCtx()
	}()

	final := r.Run(ctx)
	_ = launcher.Close()

	fmt.Printf("\nfinished in %s: %s\n", time.Since(start).Round(time.Second), final)
	if f := m.Failure(); f != nil {
		fmt.Printf("reason: %s\n", f.Message)
	}
	if final == job.StateFailed {
		return fmt.Errorf("job failed")
	}
	return nil
}
