// Package job holds the join-job state machine.
//
// The machine is deliberately pure: it never touches the clock, the disk, the
// network or the game. You feed it Inputs, it returns Transitions and Actions,
// and something else (internal/runner) actually carries the Actions out. That is
// what makes every wipe-day edge case testable in milliseconds without a game,
// a PC, or a server.
package job

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// State is where a join job currently is.
type State string

const (
	StateIdle               State = "idle"
	StateWaitingForServerUp State = "waiting_for_server_up"
	StateLaunching          State = "launching"
	StateConnecting         State = "connecting"
	StateQueued             State = "queued"
	StateInServer           State = "in_server"
	StateRetrying           State = "retrying"
	StateDone               State = "done"
	StateFailed             State = "failed"
)

// Terminal reports whether no further work will happen in this state.
func (s State) Terminal() bool { return s == StateDone || s == StateFailed }

// Config is the tuning for one job. Zero values get sensible defaults.
type Config struct {
	// WaitForServerUp arms "join as soon as the server comes back after wipe".
	// When false the job launches immediately.
	WaitForServerUp bool

	MaxAttempts int // bounded retries before we give up. Default 8.

	// RetryBackoff is the delay before attempt N. Grows, then caps.
	RetryBase time.Duration // default 5s
	RetryMax  time.Duration // default 60s

	// ConnectJitterMax spreads out our connect after we see the server come up,
	// so we are not hammering it the millisecond it opens. Default 5s.
	ConnectJitterMax time.Duration

	// MaxConnectsPerMinute caps launch attempts during wipe-day server flapping.
	// Default 4.
	MaxConnectsPerMinute int

	// InServerConfirm is how long we must stay in the server before calling the
	// job done, so a join that instantly drops is not reported as success.
	// Default 30s.
	InServerConfirm time.Duration
}

func (c *Config) applyDefaults() {
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 8
	}
	if c.RetryBase == 0 {
		c.RetryBase = 5 * time.Second
	}
	if c.RetryMax == 0 {
		c.RetryMax = 60 * time.Second
	}
	if c.ConnectJitterMax == 0 {
		c.ConnectJitterMax = 5 * time.Second
	}
	if c.MaxConnectsPerMinute == 0 {
		c.MaxConnectsPerMinute = 4
	}
	if c.InServerConfirm == 0 {
		c.InServerConfirm = 30 * time.Second
	}
}

// Inputs. Everything that can move a job along.
type Input interface{ isInput() }

type Start struct{}                                    // user pressed join, or a schedule fired
type Cancel struct{ Reason string }                    // user pressed cancel
type ServerUp struct{ Players, MaxPlayers, Queue int } // from relay-side polling
type ServerDown struct{}                               // from relay-side polling
type LaunchOK struct{}                                 // the Steam URI was handed off successfully
type LaunchFailed struct{ Reason Reason }
type LogEvent struct { // a parsed line from the Rust client log
	Kind     string
	Position int
	Detail   string
}
type GameExited struct{ Code int } // the Rust process is gone
type Tick struct{}                 // "time has passed, re-check your timers"

func (Start) isInput()        {}
func (Cancel) isInput()       {}
func (ServerUp) isInput()     {}
func (ServerDown) isInput()   {}
func (LaunchOK) isInput()     {}
func (LaunchFailed) isInput() {}
func (LogEvent) isInput()     {}
func (GameExited) isInput()   {}
func (Tick) isInput()         {}

// Reason is a failure explained in words a player understands. The Code is for
// us; the Message is what shows up on their phone.
type Reason struct {
	Code    string
	Message string
}

func (r Reason) String() string { return r.Message }

var (
	ReasonCancelled     = Reason{"cancelled", "You cancelled the join."}
	ReasonSteamProblem  = Reason{"steam_problem", "Steam isn't logged in on your PC."}
	ReasonBanned        = Reason{"rejected", "The server refused the connection."}
	ReasonGaveUp        = Reason{"gave_up", "Tried several times and couldn't get in."}
	ReasonServerFull    = Reason{"server_full", "The server is full and isn't taking a queue."}
	ReasonGameCrashed   = Reason{"game_crashed", "Rust closed unexpectedly on your PC."}
	ReasonConnectFailed = Reason{"connect_failed", "Couldn't connect to the server."}
)

// Actions are side effects the runner must perform. The machine never does them.
type Action string

const (
	ActionLaunchGame Action = "launch_game"
	ActionCloseGame  Action = "close_game"
)

// Transition is one state change, for the status feed and the phone.
type Transition struct {
	From, To State
	At       time.Time
	Detail   string  // plain language, shown to the user
	Position int     // queue position when To == queued
	Reason   *Reason // set when To == failed
	Attempt  int
}

func (t Transition) String() string {
	s := fmt.Sprintf("%s -> %s", t.From, t.To)
	if t.Detail != "" {
		s += ": " + t.Detail
	}
	return s
}

// Result is what Handle gives back.
type Result struct {
	Transitions []Transition
	Actions     []Action
}

// Machine is one join job.
//
// It is safe to use from several goroutines. That matters from phase 2 onwards:
// the relay socket delivers a cancel on one goroutine while the log tailer and
// the timers are feeding inputs on another, and the tray icon reads the current
// state on a third.
type Machine struct {
	mu  sync.Mutex
	cfg Config

	state    State
	position int
	attempt  int
	failure  *Reason

	now    func() time.Time
	jitter func(max time.Duration) time.Duration

	serverUp    bool
	serverKnown bool
	connectAt   time.Time // when we are allowed to launch (jitter / backoff)
	haveTimer   bool
	inServerAt  time.Time
	launchTimes []time.Time // for the per-minute connect cap
	lastDetail  string
}

// Option customises a Machine, mostly for tests.
type Option func(*Machine)

// WithClock replaces time.Now, so tests run instantly and deterministically.
func WithClock(now func() time.Time) Option { return func(m *Machine) { m.now = now } }

// WithJitter replaces the randomised connect delay, so tests are deterministic.
func WithJitter(j func(max time.Duration) time.Duration) Option {
	return func(m *Machine) { m.jitter = j }
}

// New builds a job in the idle state.
func New(cfg Config, opts ...Option) *Machine {
	cfg.applyDefaults()
	m := &Machine{
		cfg:   cfg,
		state: StateIdle,
		now:   time.Now,
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int63n(int64(max)))
		},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// State, Position, Attempt and Failure expose the current snapshot.
func (m *Machine) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Machine) Position() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.position
}

func (m *Machine) Attempt() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attempt
}

func (m *Machine) Failure() *Reason {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failure
}

// Snapshot is everything the phone needs to render the status screen.
type Snapshot struct {
	State    State
	Position int
	Attempt  int
	Detail   string
	Failure  *Reason
}

// Snapshot reads the whole job state consistently, in one lock.
func (m *Machine) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{
		State:    m.state,
		Position: m.position,
		Attempt:  m.attempt,
		Detail:   m.lastDetail,
		Failure:  m.failure,
	}
}

// Handle feeds one input in and reports what changed.
func (m *Machine) Handle(in Input) Result {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res Result
	if m.state.Terminal() {
		return res // nothing moves a finished job
	}

	// Cancel is honoured from any live state.
	if c, ok := in.(Cancel); ok {
		msg := c.Reason
		if msg == "" {
			msg = ReasonCancelled.Message
		}
		if m.state == StateConnecting || m.state == StateQueued || m.state == StateInServer || m.state == StateLaunching {
			res.Actions = append(res.Actions, ActionCloseGame)
		}
		res.Transitions = append(res.Transitions, m.moveTo(StateDone, msg, nil))
		return res
	}

	// Server up/down is tracked in every state; it only causes a transition
	// while we are waiting for a wipe restart.
	switch in.(type) {
	case ServerUp:
		m.serverUp, m.serverKnown = true, true
	case ServerDown:
		m.serverUp, m.serverKnown = false, true
	}

	switch m.state {
	case StateIdle:
		m.handleIdle(in, &res)
	case StateWaitingForServerUp:
		m.handleWaiting(in, &res)
	case StateLaunching:
		m.handleLaunching(in, &res)
	case StateConnecting, StateQueued:
		m.handleConnectingOrQueued(in, &res)
	case StateInServer:
		m.handleInServer(in, &res)
	case StateRetrying:
		m.handleRetrying(in, &res)
	}
	return res
}

func (m *Machine) handleIdle(in Input, res *Result) {
	if _, ok := in.(Start); !ok {
		return
	}
	if m.cfg.WaitForServerUp {
		res.Transitions = append(res.Transitions,
			m.moveTo(StateWaitingForServerUp, "Waiting for the server to come back up.", nil))
		// If we already know it is up, the next Tick will pick it up.
		if m.serverUp {
			m.armConnect()
		}
		return
	}
	m.beginLaunch(res)
}

func (m *Machine) handleWaiting(in Input, res *Result) {
	switch in.(type) {
	case ServerUp:
		// Arm a short randomised delay rather than connecting instantly. During a
		// wipe restart the server flaps; a tiny wait avoids a pointless connect
		// into a server that is only half up, and staggers us against everyone else.
		if !m.haveTimer {
			m.armConnect()
		}
	case ServerDown:
		// It went down again before we fired. Disarm and keep waiting. This is the
		// flap tolerance: no giving up, no attempt burned.
		m.haveTimer = false
		m.lastDetail = "Server went down again, still waiting."
	case Tick:
		if m.haveTimer && !m.now().Before(m.connectAt) && m.serverUp {
			m.haveTimer = false
			if m.rateLimited() {
				return // hold; a later Tick will let us through
			}
			m.beginLaunch(res)
		}
	}
}

func (m *Machine) handleLaunching(in Input, res *Result) {
	switch v := in.(type) {
	case LaunchOK:
		res.Transitions = append(res.Transitions,
			m.moveTo(StateConnecting, "Rust is starting and connecting.", nil))
	case LaunchFailed:
		m.retryOrFail(v.Reason, res)
	case LogEvent:
		m.handleLogEvent(v, res)
	}
}

func (m *Machine) handleConnectingOrQueued(in Input, res *Result) {
	switch v := in.(type) {
	case LogEvent:
		m.handleLogEvent(v, res)
	case GameExited:
		// The client died while we were connecting or queuing. Relaunch.
		m.retryOrFail(ReasonGameCrashed, res)
	}
}

func (m *Machine) handleInServer(in Input, res *Result) {
	switch v := in.(type) {
	case LogEvent:
		if v.Kind == "disconnected" || v.Kind == "rejected" {
			m.retryOrFail(Reason{"dropped", "You were dropped from the server."}, res)
		}
	case GameExited:
		m.retryOrFail(ReasonGameCrashed, res)
	case Tick:
		// Only call it a win once we have held the slot for a bit.
		if !m.now().Before(m.inServerAt.Add(m.cfg.InServerConfirm)) {
			res.Transitions = append(res.Transitions,
				m.moveTo(StateDone, "You're in. Slot is being held.", nil))
		}
	}
}

func (m *Machine) handleRetrying(in Input, res *Result) {
	if _, ok := in.(Tick); !ok {
		return
	}
	if m.now().Before(m.connectAt) {
		return
	}
	if m.cfg.WaitForServerUp && m.serverKnown && !m.serverUp {
		res.Transitions = append(res.Transitions,
			m.moveTo(StateWaitingForServerUp, "Server is down, waiting for it to come back.", nil))
		return
	}
	if m.rateLimited() {
		return
	}
	m.beginLaunch(res)
}

// handleLogEvent maps a parsed Rust log line onto the state machine.
func (m *Machine) handleLogEvent(v LogEvent, res *Result) {
	switch v.Kind {
	case "steam_problem":
		// Never retry this one. Retrying cannot fix a logged-out Steam, and the
		// user needs to be told the actual problem.
		m.fail(ReasonSteamProblem, res)
	case "connecting":
		if m.state != StateConnecting {
			res.Transitions = append(res.Transitions, m.moveTo(StateConnecting, v.Detail, nil))
		}
	case "queued":
		if m.state == StateQueued && v.Position == m.position {
			return // no change worth reporting
		}
		m.position = v.Position
		res.Transitions = append(res.Transitions, m.moveTo(StateQueued, v.Detail, nil))
	case "joined":
		if m.state == StateInServer {
			return
		}
		m.inServerAt = m.now()
		m.position = 0
		res.Transitions = append(res.Transitions, m.moveTo(StateInServer, v.Detail, nil))
	case "server_full":
		m.retryOrFail(ReasonServerFull, res)
	case "rejected":
		r := ReasonBanned
		if v.Detail != "" {
			r.Message = v.Detail
		}
		m.fail(r, res) // a refusal will not fix itself; do not retry
	case "disconnected":
		r := ReasonConnectFailed
		if v.Detail != "" {
			r.Message = v.Detail
		}
		m.retryOrFail(r, res)
	}
}

// beginLaunch records an attempt and asks the runner to launch the game.
func (m *Machine) beginLaunch(res *Result) {
	m.attempt++
	m.launchTimes = append(m.launchTimes, m.now())
	m.position = 0
	res.Transitions = append(res.Transitions,
		m.moveTo(StateLaunching, fmt.Sprintf("Launching Rust (attempt %d).", m.attempt), nil))
	res.Actions = append(res.Actions, ActionLaunchGame)
}

// retryOrFail backs off and tries again, unless we are out of attempts.
func (m *Machine) retryOrFail(r Reason, res *Result) {
	if m.attempt >= m.cfg.MaxAttempts {
		m.fail(ReasonGaveUp, res)
		return
	}
	back := m.backoff()
	m.connectAt = m.now().Add(back)
	res.Transitions = append(res.Transitions,
		m.moveTo(StateRetrying, fmt.Sprintf("%s Trying again in %s.", r.Message, back.Round(time.Second)), nil))
	res.Actions = append(res.Actions, ActionCloseGame)
}

func (m *Machine) fail(r Reason, res *Result) {
	m.failure = &r
	res.Transitions = append(res.Transitions, m.moveTo(StateFailed, r.Message, &r))
	res.Actions = append(res.Actions, ActionCloseGame)
}

// backoff doubles from RetryBase, capped at RetryMax.
func (m *Machine) backoff() time.Duration {
	d := m.cfg.RetryBase
	for i := 1; i < m.attempt; i++ {
		d *= 2
		if d >= m.cfg.RetryMax {
			return m.cfg.RetryMax
		}
	}
	return d
}

// armConnect sets the short randomised delay before connecting after we see the
// server come up.
func (m *Machine) armConnect() {
	m.connectAt = m.now().Add(m.jitter(m.cfg.ConnectJitterMax))
	m.haveTimer = true
}

// rateLimited stops us hammering a flapping server on wipe day.
func (m *Machine) rateLimited() bool {
	cutoff := m.now().Add(-time.Minute)
	var recent []time.Time
	for _, t := range m.launchTimes {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	m.launchTimes = recent
	return len(recent) >= m.cfg.MaxConnectsPerMinute
}

func (m *Machine) moveTo(s State, detail string, r *Reason) Transition {
	t := Transition{
		From: m.state, To: s, At: m.now(),
		Detail: detail, Position: m.position, Reason: r, Attempt: m.attempt,
	}
	m.state = s
	m.lastDetail = detail
	return t
}
