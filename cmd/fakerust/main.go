// Command fakerust pretends to be the Rust client.
//
// It is useful on its own for eyeballing a scenario:
//
//	go run ./cmd/fakerust --scenario testdata/scenarios/long_queue.json --log /tmp/Player.log
//
// The agent in --sim mode runs the same code in-process, so there is nothing to
// build or install first.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"queueup/internal/fakerust"
	"queueup/internal/scenario"
)

func main() {
	var (
		scenarioPath = flag.String("scenario", "", "path to a scenario JSON file (required)")
		logPath      = flag.String("log", "", "fake Player.log to write (required)")
		connect      = flag.String("connect", "127.0.0.1:28015", "server address to pretend to connect to")
		speed        = flag.Float64("speed", 1, "timeline speed multiplier; 10 runs ten times faster")
	)
	flag.Parse()

	if *scenarioPath == "" || *logPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	sc, err := scenario.Load(*scenarioPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("fake rust: scenario %q writing to %s\n", sc.Name, *logPath)
	err = fakerust.Run(ctx, fakerust.Config{
		Scenario: sc,
		LogPath:  *logPath,
		Connect:  *connect,
		Speed:    *speed,
		OnLine:   func(l string) { fmt.Println("  log |", l) },
	})
	var ee fakerust.ExitError
	if errors.As(err, &ee) {
		fmt.Println("fake rust: simulated crash")
		os.Exit(ee.Code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
