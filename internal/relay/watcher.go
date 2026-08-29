package relay

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"queueup/internal/a2s"
	"queueup/internal/protocol"
	"queueup/internal/store"
)

// Watcher polls the target server of every running job and streams what it
// sees down to the agent and into the job timeline.
//
// It runs on the relay, not the PC, for two reasons: the relay has one clock
// and one view of the truth for the phone to read, and on wipe day the polling
// must carry on even while the PC is busy relaunching Rust.
//
// This is the wipe-restart detector. While a job is waiting for the server to
// come back, its server is polled hard (every couple of seconds), so the down
// to up flip is spotted within seconds and the agent told immediately. That
// message is what beats a human refreshing the server browser.
type Watcher struct {
	Store *store.Store
	Hub   *Hub
	Log   *slog.Logger

	// Query is swappable for tests. Defaults to a real A2S query.
	Query func(ctx context.Context, addr string) (a2s.Info, error)

	// WaitingPoll is the interval while a job is waiting for a wipe restart.
	// OtherPoll covers every other active state, where the numbers are nice to
	// have but nothing hangs on them.
	WaitingPoll time.Duration
	OtherPoll   time.Duration

	mu   sync.Mutex
	seen map[string]bool // job id -> last known online, for spotting the flip
	last map[string]time.Time
}

// prepare fills in the defaults. Separate from Run so that a sweep can be
// exercised on its own.
func (w *Watcher) prepare() {
	if w.Log == nil {
		w.Log = slog.Default()
	}
	if w.WaitingPoll <= 0 {
		w.WaitingPoll = 2 * time.Second
	}
	if w.OtherPoll <= 0 {
		w.OtherPoll = 15 * time.Second
	}
	if w.Query == nil {
		w.Query = func(ctx context.Context, addr string) (a2s.Info, error) {
			return a2s.Query(ctx, addr, 2*time.Second)
		}
	}
	w.seen = map[string]bool{}
	w.last = map[string]time.Time{}
}

// Run polls until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	w.prepare()

	tick := time.NewTicker(w.WaitingPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.sweep(ctx)
		}
	}
}

func (w *Watcher) sweep(ctx context.Context) {
	jobs, err := w.Store.ActiveJobs()
	if err != nil {
		w.Log.Error("listing active jobs", "err", err)
		return
	}

	w.mu.Lock()
	// Drop state for jobs that have finished, so the maps cannot grow forever.
	live := map[string]bool{}
	for _, j := range jobs {
		live[j.ID] = true
	}
	for id := range w.seen {
		if !live[id] {
			delete(w.seen, id)
			delete(w.last, id)
		}
	}
	w.mu.Unlock()

	// Group the jobs that are due by the address they need asking about.
	//
	// On wipe day most of these are the SAME address: everybody is waiting for
	// the same popular server. Asking it once per waiting player would be slow
	// and rude, and a server hammered by the queue tool is a server that blocks
	// the queue tool.
	due := map[string][]store.Job{}
	for _, j := range jobs {
		if !w.due(j) {
			continue
		}
		addr := j.PollAddr()
		due[addr] = append(due[addr], j)
	}
	if len(due) == 0 {
		return
	}

	// Ask them all at once. A server that is DOWN does not refuse the question,
	// it just never answers, so every poll during a wipe restart costs the full
	// timeout. Serially, the sweep would take servers x timeout, and the restart
	// would be spotted minutes late on the one day that matters.
	const maxParallel = 32
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for addr, group := range due {
		wg.Add(1)
		go func(addr string, group []store.Job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			info, err := w.Query(qctx, addr)
			cancel()

			for _, j := range group {
				w.applyStatus(j, info, err == nil)
			}
		}(addr, group)
	}
	wg.Wait()
}

// due reports whether this job is ready for another poll, and records that we
// are about to make one.
func (w *Watcher) due(j store.Job) bool {
	interval := w.OtherPoll
	// Fast polling while the answer is what somebody is actively watching:
	// waiting for a wipe restart, and sitting in a queue. Rust does not log
	// queue positions, so the server's own count IS the queue display, and a
	// countdown that updates every 15 seconds reads as broken.
	if waitingForRestart(j) || j.State == "connecting" || j.State == "queued" {
		interval = w.WaitingPoll
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if last, polled := w.last[j.ID]; polled && time.Since(last) < interval {
		return false
	}
	w.last[j.ID] = time.Now()
	return true
}

// waitingForRestart is true in the states where the server coming back is the
// thing we are waiting on, and so worth polling hard.
func waitingForRestart(j store.Job) bool {
	return j.State == "waiting_for_server_up" || j.State == "pending" || j.State == "retrying"
}

// applyStatus tells one job's agent what we saw, and records the flips.
func (w *Watcher) applyStatus(j store.Job, info a2s.Info, online bool) {
	w.mu.Lock()
	wasOnline, known := w.seen[j.ID]
	w.seen[j.ID] = online
	w.mu.Unlock()

	// Tell the agent every time. The agent's own jitter and rate cap decide what
	// to do with it; the relay just reports what it sees, fast.
	st := protocol.ServerStatus{JobID: j.ID, Online: online}
	if online {
		st.Players, st.MaxPlayers, st.Queue = info.Players, info.MaxPlayers, info.Queue
	}
	if err := w.Hub.SendTo(j.DeviceID, protocol.TypeServerStatus, st); err != nil {
		// Agent offline; it will hear the current state as soon as it returns.
		_ = err
	}

	// The timeline only records the flips, and only while they matter.
	if known && online != wasOnline && waitingForRestart(j) {
		if online {
			w.note(j, "The server is back up. Connecting now.")
			w.Log.Info("server came back", "job", j.ID, "addr", j.PollAddr(),
				"players", info.Players, "queue", info.Queue)
		} else {
			w.note(j, "The server went down. This is normal during a wipe restart; watching for it to come back.")
		}
	}
}

func (w *Watcher) note(j store.Job, detail string) {
	now := time.Now().UTC()
	if err := w.Store.AppendEvent(j.ID, j.State, 0, detail, now); err != nil {
		w.Log.Error("appending watcher event", "err", err)
		return
	}
	events, err := w.Store.Events(j.ID, 0)
	if err == nil && len(events) > 0 {
		w.Hub.Publish(events[len(events)-1])
	}
}
