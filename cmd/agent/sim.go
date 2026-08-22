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

// cmdSim is the phase 1 command: run one join on this machine and print what is
// happening, with no relay and no web app involved. It is still the fastest way
// to see the whole flow, and it is what the scenario demo script uses.
func cmdSim(args []string) error {
	fs := flag.NewFlagSet("sim", flag.ExitOnError)
	var (
		useSim       = fs.Bool("sim", true, "use the fake Rust simulator instead of the real game")
		scenarioPath = fs.String("scenario", "", "scenario file to simulate (required)")
		serverAddr   = fs.String("server", "127.0.0.1:28015", "target server as IP:PORT")
		patternsPath = fs.String("patterns", "configs/patterns.json", "log pattern file")
		logPath      = fs.String("log", "", "Rust log file to watch (default: a temp file)")
		waitForUp    = fs.Bool("wait-for-server-up", false, "wait for the server to come back after a wipe restart")
		speed        = fs.Float64("speed", 1, "simulator timeline speed multiplier")
		jitter       = fs.Duration("jitter", time.Second, "maximum random delay before connecting after the server comes up")
		maxAttempts  = fs.Int("max-attempts", 8, "how many times to try before giving up")
		confirm      = fs.Duration("confirm", 5*time.Second, "how long to hold a slot before calling the job done")
		showAllLines = fs.Bool("verbose", false, "print every log line, not just the ones we understand")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runSim(*useSim, *scenarioPath, *serverAddr, *patternsPath, *logPath, *waitForUp,
		*speed, *jitter, *maxAttempts, *confirm, *showAllLines)
}

func runSim(sim bool, scenarioPath, serverAddr, patternsPath, logPath string, waitForUp bool,
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
