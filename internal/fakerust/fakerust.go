// Package fakerust is a stand-in for the Rust client.
//
// From the agent's point of view it behaves exactly like the real game: it is a
// process you start, it writes lines into a Player.log on a timeline, it can be
// asked to close, and it can die unexpectedly. It knows nothing about the agent
// and the agent knows nothing special about it. That is the point: all of the
// development and every automated test runs against this, with no Rust, no
// Steam, and no Windows PC involved.
package fakerust

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"queueup/internal/scenario"
)

// Config is how to run one fake session.
type Config struct {
	Scenario *scenario.Scenario
	LogPath  string       // the fake Player.log to write
	Connect  string       // IP:PORT, substituted into {{connect}} in scenario lines
	Speed    float64      // 1.0 = real time; 10 = ten times faster, for tests
	OnLine   func(string) // optional, for tests and the console
}

// ExitError reports a scenario that ended with a simulated crash.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("fake rust exited with code %d", e.Code) }

// Run plays a scenario. It returns nil when the scenario finishes normally or
// the context is cancelled (a graceful close), and an ExitError when the
// scenario scripts a crash.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Speed <= 0 {
		cfg.Speed = 1
	}
	// The real client truncates its log on launch. Do the same, so we exercise
	// the tailer's "file got shorter, start over" path.
	f, err := os.Create(cfg.LogPath)
	if err != nil {
		return fmt.Errorf("creating fake log: %w", err)
	}
	defer f.Close()

	write := func(line string) error {
		stamped := fmt.Sprintf("%s %s\n", time.Now().Format("15:04:05"), line)
		if _, err := f.WriteString(stamped); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		if cfg.OnLine != nil {
			cfg.OnLine(line)
		}
		return nil
	}

	if err := write("Fake Rust client started (this is the QueueUp simulator, not the real game)"); err != nil {
		return err
	}

	for _, step := range cfg.Scenario.Steps {
		d := time.Duration(float64(step.Delay()) / cfg.Speed)
		select {
		case <-ctx.Done():
			_ = write("Shutting down")
			return nil
		case <-time.After(d):
		}
		if step.Exit != nil {
			_ = write(fmt.Sprintf("Simulated crash (exit code %d)", *step.Exit))
			if *step.Exit == 0 {
				return nil
			}
			return ExitError{Code: *step.Exit}
		}
		if step.Line == "" {
			continue
		}
		if err := write(strings.ReplaceAll(step.Line, "{{connect}}", cfg.Connect)); err != nil {
			return err
		}
	}

	// Scenario finished with no crash: behave like a client sitting in the game,
	// alive until someone closes it.
	<-ctx.Done()
	_ = write("Shutting down")
	return nil
}
