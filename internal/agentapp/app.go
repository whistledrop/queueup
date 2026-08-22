// Package agentapp is the agent's job runner: it takes orders from the relay and
// turns them into actual join attempts, reporting every step back up.
//
// Splitting this out of the command line means the whole thing, relay included,
// can be exercised in a test.
package agentapp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/protocol"
	"queueup/internal/relayclient"
	"queueup/internal/runner"
	"queueup/internal/serverstat"
)

// LauncherFactory builds the thing that will start the game for one job. In
// simulator mode it returns a fake; on a real PC it returns the Windows
// launcher. A factory rather than a single launcher because each job gets a
// clean one.
type LauncherFactory func(j protocol.Job) (game.Launcher, error)

// App runs at most one job at a time. One account, one PC, one job: two jobs
// would be two things fighting over the same copy of Rust.
type App struct {
	Client    *relayclient.Client
	Parser    *logparse.Parser
	NewGame   LauncherFactory
	Log       *slog.Logger
	JobConfig job.Config

	// SendLogLines mirrors raw Rust log lines to the relay for the debug view.
	SendLogLines bool

	mu      sync.Mutex
	current *activeJob
}

type activeJob struct {
	id     string
	cancel context.CancelFunc
	runner *runner.Runner
	feed   *serverstat.Feed
	done   chan struct{}
}

// OnConnected is called each time the socket comes up.
func (a *App) OnConnected(w protocol.Welcome) {
	a.Log.Info("relay connection is up", "device", w.DeviceID)
}

// OnDisconnected is called when it goes down. Nothing is torn down here on
// purpose: if the relay disappears mid-queue we keep sitting in the queue, and
// catch the relay up when it returns.
func (a *App) OnDisconnected(err error) {
	a.Log.Warn("relay connection is down; the current join keeps running", "err", err)
}

// OnServerStatus passes the relay's view of the target server to the running job.
func (a *App) OnServerStatus(st protocol.ServerStatus) {
	a.mu.Lock()
	cur := a.current
	a.mu.Unlock()
	if cur == nil || (st.JobID != "" && st.JobID != cur.id) {
		return
	}
	cur.feed.Push(serverstat.Status{
		Online: st.Online, Players: st.Players,
		MaxPlayers: st.MaxPlayers, Queue: st.Queue, At: time.Now(),
	})
}

// OnCancel stops the running job and closes the game.
//
// This goes through the state machine rather than just killing the job, so the
// job is properly reported as finished. Compare Stop below, which does the
// opposite on purpose.
func (a *App) OnCancel(c protocol.Cancel) {
	a.mu.Lock()
	cur := a.current
	a.mu.Unlock()
	if cur == nil || (c.JobID != "" && c.JobID != cur.id) {
		return
	}
	reason := c.Reason
	if reason == "" {
		reason = "The join was cancelled."
	}
	a.Log.Info("cancelling job", "job", cur.id)
	cur.runner.Cancel(reason)
	cur.cancel()
}

// OnAssign starts a job, or resumes one after a reconnection or a reboot.
func (a *App) OnAssign(j protocol.Job) {
	a.mu.Lock()
	if a.current != nil && a.current.id == j.ID {
		// The relay hands the job back every time we reconnect. If we are already
		// on it, carry on rather than starting over and losing our queue place.
		a.mu.Unlock()
		a.Log.Info("already working on this job, carrying on", "job", j.ID)
		return
	}
	previous := a.current
	a.mu.Unlock()

	if previous != nil {
		a.Log.Info("a different job arrived, stopping the current one", "was", previous.id, "now", j.ID)
		previous.cancel()
		<-previous.done
	}

	addr, err := game.ParseAddr(j.ServerAddr)
	if err != nil {
		a.report(protocol.JobStatus{
			JobID: j.ID, State: string(job.StateFailed),
			Detail:        "That server address doesn't look right.",
			ReasonCode:    "bad_address",
			ReasonMessage: "That server address doesn't look right.",
			At:            time.Now().UTC(),
		})
		return
	}

	launcher, err := a.NewGame(j)
	if err != nil {
		a.report(protocol.JobStatus{
			JobID: j.ID, State: string(job.StateFailed),
			Detail: err.Error(), ReasonCode: "launcher_unavailable",
			ReasonMessage: err.Error(), At: time.Now().UTC(),
		})
		return
	}

	cfg := a.JobConfig
	cfg.WaitForServerUp = j.WaitForServerUp

	feed := serverstat.NewFeed()
	if !j.WaitForServerUp {
		// Nothing is waiting on the server's state, so do not make the job sit
		// around for a status message that has no bearing on it.
		feed.Push(serverstat.Status{Online: true})
	}

	ctx, cancel := context.WithCancel(context.Background())
	act := &activeJob{id: j.ID, cancel: cancel, feed: feed, done: make(chan struct{})}

	m := job.New(cfg)
	r := &runner.Runner{
		Machine:  m,
		Launcher: launcher,
		Parser:   a.Parser,
		Server:   feed,
		Addr:     addr,
		OnTransition: func(t job.Transition) {
			st := protocol.JobStatus{
				JobID: j.ID, State: string(t.To), Position: t.Position,
				Attempt: t.Attempt, Detail: t.Detail, At: t.At.UTC(),
			}
			if t.Reason != nil {
				st.ReasonCode, st.ReasonMessage = t.Reason.Code, t.Reason.Message
			}
			a.report(st)
		},
		OnLogLine: func(line string, understood bool) {
			if a.SendLogLines {
				a.Client.Send(protocol.TypeJobLog, protocol.JobLog{
					JobID: j.ID, Line: line, At: time.Now().UTC(),
				})
			}
		},
	}

	act.runner = r

	a.mu.Lock()
	a.current = act
	a.mu.Unlock()

	if j.Resumed {
		a.Log.Info("resuming a job after reconnecting", "job", j.ID)
	}

	go func() {
		defer close(act.done)
		defer cancel()

		final := r.Run(ctx)
		_ = launcher.Close()

		// A job stopped by a cancel command has already been reported as done by
		// the state machine. A job stopped because the agent is shutting down has
		// not, and the relay will hand it back when we return.
		a.Log.Info("job finished", "job", j.ID, "state", final)

		a.mu.Lock()
		if a.current == act {
			a.current = nil
		}
		a.mu.Unlock()
	}()
}

// Stop ends the running job WITHOUT reporting it as finished, so that the relay
// hands it straight back when the agent starts up again. This is what a clean
// shutdown does, and it is exactly what a surprise Windows reboot looks like
// from the relay's side.
func (a *App) Stop() {
	a.mu.Lock()
	cur := a.current
	a.mu.Unlock()
	if cur == nil {
		return
	}
	cur.cancel()
	<-cur.done
}

// CurrentJobID is the job in progress, if any.
func (a *App) CurrentJobID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current == nil {
		return ""
	}
	return a.current.id
}

func (a *App) report(st protocol.JobStatus) {
	a.Log.Info("job status", "job", st.JobID, "state", st.State, "detail", st.Detail)
	a.Client.Send(protocol.TypeJobStatus, st)
}
