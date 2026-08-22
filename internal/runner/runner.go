// Package runner wires the pieces together and actually runs a join job.
//
// The state machine decides WHAT should happen. This file makes it happen: it
// tails the log, polls the server, launches and closes the game, and feeds
// everything back in as inputs. Keeping the two apart is what lets us test all
// the nasty wipe-day cases without a game.
package runner

import (
	"context"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/logtail"
	"queueup/internal/serverstat"
)

// Runner executes one join job to completion.
type Runner struct {
	Machine  *job.Machine
	Launcher game.Launcher
	Parser   *logparse.Parser
	Server   serverstat.Source
	Addr     game.Addr

	// Tick is how often timers are re-checked, and ServerPoll how often the
	// server is checked. Both get sensible defaults.
	Tick       time.Duration
	ServerPoll time.Duration
	LogPoll    time.Duration

	// OnTransition is called for every state change. Phase 1 prints it to the
	// console; phase 2 will also push it up the relay socket.
	OnTransition func(job.Transition)
	// OnLogLine is optional, for the debug log viewer.
	OnLogLine func(string, bool)
}

type launchResult struct {
	err error
}

// Run drives the job until it reaches done or failed, or ctx is cancelled.
func (r *Runner) Run(ctx context.Context) job.State {
	if r.Tick == 0 {
		r.Tick = 500 * time.Millisecond
	}
	if r.ServerPoll == 0 {
		r.ServerPoll = 2 * time.Second
	}
	if r.LogPoll == 0 {
		r.LogPoll = 200 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan string, 256)
	// false: ignore whatever is already in the log from the previous Rust
	// session. The game truncates it on launch and the tailer reads the new
	// session from the top.
	go logtail.Follow(ctx, r.Launcher.LogPath(), r.LogPoll, false, func(l string) {
		select {
		case lines <- l:
		case <-ctx.Done():
		}
	})

	statuses := make(chan serverstat.Status, 8)
	if r.Server != nil {
		go func() {
			t := time.NewTicker(r.ServerPoll)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					select {
					case statuses <- r.Server.Poll():
					default:
					}
				}
			}
		}()
	}

	launches := make(chan launchResult, 4)
	ticker := time.NewTicker(r.Tick)
	defer ticker.Stop()

	// When the game dies we do not report it straight away. The log line that
	// explains WHY it died ("Steam isn't logged in") is usually written a moment
	// before the process disappears, and the tailer polls, so the exit can
	// overtake it. A short grace period means the user gets the real reason
	// instead of a useless "Rust closed unexpectedly".
	exitGrace := 4 * r.LogPoll
	var exitTimer <-chan time.Time
	var pendingExit game.Exit
	exits := r.Launcher.Exited()

	r.feed(job.Start{}, launches, cancel)

	for !r.Machine.State().Terminal() {
		select {
		case <-ctx.Done():
			return r.Machine.State()

		case <-ticker.C:
			r.feed(job.Tick{}, launches, cancel)

		case st := <-statuses:
			if st.Online {
				r.feed(job.ServerUp{Players: st.Players, MaxPlayers: st.MaxPlayers, Queue: st.Queue}, launches, cancel)
			} else {
				r.feed(job.ServerDown{}, launches, cancel)
			}

		case line := <-lines:
			ev, ok := r.Parser.ParseLine(line)
			if r.OnLogLine != nil {
				r.OnLogLine(line, ok)
			}
			if ok {
				r.feed(job.LogEvent{Kind: string(ev.Kind), Position: ev.Position, Detail: ev.Detail}, launches, cancel)
			}

		case lr := <-launches:
			if lr.err != nil {
				r.feed(job.LaunchFailed{Reason: job.Reason{Code: "launch_failed", Message: lr.err.Error()}}, launches, cancel)
			} else {
				r.feed(job.LaunchOK{}, launches, cancel)
			}

		case ex := <-exits:
			if !ex.Expected {
				pendingExit = ex
				exitTimer = time.After(exitGrace)
			}

		case <-exitTimer:
			exitTimer = nil
			r.feed(job.GameExited{Code: pendingExit.Code}, launches, cancel)
		}
	}
	return r.Machine.State()
}

// Cancel stops a running job the way the user's cancel button will.
func (r *Runner) Cancel(reason string) {
	res := r.Machine.Handle(job.Cancel{Reason: reason})
	for _, t := range res.Transitions {
		if r.OnTransition != nil {
			r.OnTransition(t)
		}
	}
	_ = r.Launcher.Close()
}

// feed pushes one input into the machine and carries out whatever it asks for.
func (r *Runner) feed(in job.Input, launches chan launchResult, cancel context.CancelFunc) {
	res := r.Machine.Handle(in)
	for _, t := range res.Transitions {
		if r.OnTransition != nil {
			r.OnTransition(t)
		}
	}
	for _, a := range res.Actions {
		switch a {
		case job.ActionLaunchGame:
			go func() {
				if err := r.Launcher.Preflight(); err != nil {
					launches <- launchResult{err: err}
					return
				}
				launches <- launchResult{err: r.Launcher.Launch(r.Addr)}
			}()
		case job.ActionCloseGame:
			go func() { _ = r.Launcher.Close() }()
		}
	}
}
