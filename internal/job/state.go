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
type GameExited struct {
	Code int
	// Reason, when set, is why, in words for the player.
	Reason string
}                  // the Rust process is gone
type Tick struct{} // "time has passed, re-check your timers"

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

// steamProblemRetries is how many times a Steam failure is treated as a blip
// before it is treated as the truth.
const steamProblemRetries = 2

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

	serverUp      bool
	serverKnown   bool
	steamProblems int

	// loadingSeen records that the game has moved past the queue into loading
	// the world. From then on the server's queue count describes other people,
	// and must not drag the display back into "queued".
	loadingSeen bool

	// logQueuePosition records that the game's own log reported a queue
	// position this launch. When it does, that number is the player's actual
	// place in line and outranks the server's coarser "how many are queued"
	// count, so server updates stand down for the rest of the launch.
	logQueuePosition bool

	// sawUserQuit records that the game announced a graceful shutdown that WE
	// did not ask for: the player closed Rust themselves. closeRequested is what
	// tells those apart. When the machine orders the game closed (a retry, a
	// failure, a cancel), the same farewell lines appear in the log, and without
	// this flag every retry would read as the player quitting.
	sawUserQuit    bool
	closeRequested bool
	connectAt      time.Time // when we are allowed to launch (jitter / backoff)
	haveTimer      bool
	inServerAt     time.Time
	launchTimes    []time.Time // for the per-minute connect cap
	lastDetail     string
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
		reason := ReasonCancelled
		if c.Reason != "" {
			reason.Message = c.Reason
		}
		if m.state == StateConnecting || m.state == StateQueued || m.state == StateInServer || m.state == StateLaunching {
			m.closeRequested = true
			res.Actions = append(res.Actions, ActionCloseGame)
		}
		// The reason is carried even though this is not a failure, so that the
		// phone can tell "you cancelled it" apart from "you got in". Both end up
		// in the done state, and they should not look the same.
		res.Transitions = append(res.Transitions, m.moveTo(StateDone, reason.Message, &reason))
		return res
	}

	// Server up/down is tracked in every state; it only causes a transition
	// while we are waiting for a wipe restart.
	switch v := in.(type) {
	case ServerUp:
		m.serverUp, m.serverKnown = true, true
	case ServerDown:
		m.serverUp, m.serverKnown = false, true
	case LogEvent:
		// The player closing the game is worth remembering whatever state we
		// are in, but only when the shutdown was not one we ordered.
		if v.Kind == "user_quit" && !m.closeRequested {
			m.sawUserQuit = true
		}
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
	case ServerUp:
		m.handleServerQueue(v, res)
	case LogEvent:
		m.handleLogEvent(v, res)
	case GameExited:
		if m.sawUserQuit {
			// The player closed Rust themselves. That is their answer, and
			// relaunching the game in their face is not taking it. Stop cleanly.
			m.playerClosed(res)
			return
		}
		// The client died while we were connecting or queuing. Relaunch.
		m.retryOrFail(exitReason(v), res)
	}
}

// exitReason prefers whatever the launcher knew over the generic crash text.
// "Steam stopped downloading Rust" is a great deal more use to somebody staring
// at their phone than "Rust closed unexpectedly".
func exitReason(v GameExited) Reason {
	if v.Reason != "" {
		return Reason{Code: "game_unavailable", Message: v.Reason}
	}
	return ReasonGameCrashed
}

func (m *Machine) handleInServer(in Input, res *Result) {
	switch v := in.(type) {
	case LogEvent:
		if v.Kind == "disconnected" || v.Kind == "rejected" {
			m.retryOrFail(Reason{"dropped", "You were dropped from the server."}, res)
		}
	case GameExited:
		if m.sawUserQuit {
			m.playerClosed(res)
			return
		}
		m.retryOrFail(exitReason(v), res)
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
		// Steam restarts itself when it updates, and force wipe is exactly when
		// it does. Rust launched during that window can fail to reach Steam for
		// a few seconds, so the first sightings get a quick retry.
		//
		// A Steam that is genuinely signed out is not fixed by waiting, so this
		// concludes fast rather than spending the whole retry budget on it.
		m.steamProblems++
		if m.steamProblems <= steamProblemRetries {
			m.retryOrFail(ReasonSteamProblem, res)
			return
		}
		m.fail(ReasonSteamProblem, res)
	case "connecting":
		if m.state != StateConnecting {
			res.Transitions = append(res.Transitions, m.moveTo(StateConnecting, v.Detail, nil))
		}
	case "queued":
		m.logQueuePosition = true
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
	case "loading":
		// Past the queue, into the world. This is what retires the queue
		// display: from here the server's count is other people's queue.
		if m.loadingSeen {
			return
		}
		m.loadingSeen = true
		if m.state == StateQueued || m.state == StateConnecting {
			m.position = 0
			res.Transitions = append(res.Transitions, m.moveTo(StateConnecting, v.Detail, nil))
		}
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
	// A fresh launch is a fresh game: whatever the previous copy said on its
	// way out no longer applies.
	m.sawUserQuit, m.closeRequested = false, false
	m.logQueuePosition, m.loadingSeen = false, false
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
	m.closeRequested = true
	res.Transitions = append(res.Transitions,
		m.moveTo(StateRetrying, fmt.Sprintf("%s Trying again in %s.", r.Message, back.Round(time.Second)), nil))
	res.Actions = append(res.Actions, ActionCloseGame)
}

func (m *Machine) fail(r Reason, res *Result) {
	m.closeRequested = true
	m.failure = &r
	res.Transitions = append(res.Transitions, m.moveTo(StateFailed, r.Message, &r))
	res.Actions = append(res.Actions, ActionCloseGame)
}

// handleServerQueue turns the server's own "how many are in line" answer into
// the queue display, because Rust does not log queue positions at all
// (verified against a real full-server session, 2026-08-29). This number is
// the LENGTH of the line the player is standing in, not their exact place, and
// the wording is careful to say so. A position from the game's own log, if one
// ever appears, outranks it.
func (m *Machine) handleServerQueue(v ServerUp, res *Result) {
	if m.logQueuePosition || m.loadingSeen {
		return
	}
	q := v.Queue
	switch m.state {
	case StateConnecting:
		if q <= 0 {
			return // no line; the connect is just taking its time
		}
	case StateQueued:
		// The estimate only ever moves toward the front. Your place cannot be
		// worse than the whole line, and people joining BEHIND you grow the
		// line without moving you, so a bigger count than before means nothing
		// about you and must not push your number back up.
		if q >= m.position {
			return
		}
	default:
		return
	}
	m.position = q
	res.Transitions = append(res.Transitions, m.moveTo(StateQueued, queueDetail(q), nil))
}

// queueDetail says where the player is, in words for the phone. The number is
// an estimate from the server's queue length, and the wording says so: Rust
// tells nobody their exact place outside the game.
func queueDetail(q int) string {
	switch {
	case q <= 0:
		return "At the front of the queue."
	case q == 1:
		return "Almost in: next in the queue."
	default:
		return fmt.Sprintf("In the queue, about position %d.", q)
	}
}

// playerClosed ends the job because the player shut the game themselves,
// perhaps to play something else. It reads as a cancellation on the phone, not
// a failure: nothing went wrong, the person changed their mind at the keyboard
// instead of in the app.
func (m *Machine) playerClosed(res *Result) {
	r := Reason{Code: "player_closed", Message: "Rust was closed on your PC, so QueueUp stopped this join."}
	res.Transitions = append(res.Transitions, m.moveTo(StateDone, r.Message, &r))
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
