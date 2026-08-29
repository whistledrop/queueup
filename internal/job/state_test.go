package job

import (
	"strings"
	"testing"
	"time"
)

// testClock lets every timing case run instantly and identically every time.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestMachine(cfg Config) (*Machine, *testClock) {
	c := &testClock{t: time.Date(2026, 8, 22, 18, 0, 0, 0, time.UTC)}
	m := New(cfg,
		WithClock(c.now),
		WithJitter(func(max time.Duration) time.Duration { return max / 2 }), // deterministic
	)
	return m, c
}

// feed pushes inputs in and returns the states we passed through.
func feed(m *Machine, ins ...Input) []State {
	var states []State
	for _, in := range ins {
		for _, tr := range m.Handle(in).Transitions {
			states = append(states, tr.To)
		}
	}
	return states
}

func actionsFor(m *Machine, in Input) []Action { return m.Handle(in).Actions }

func TestHappyPathThroughQueue(t *testing.T) {
	m, c := newTestMachine(Config{InServerConfirm: 30 * time.Second})

	if got := m.State(); got != StateIdle {
		t.Fatalf("start state = %s, want idle", got)
	}
	if acts := actionsFor(m, Start{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("Start should ask the runner to launch the game, got %v", acts)
	}
	if m.State() != StateLaunching {
		t.Fatalf("state = %s, want launching", m.State())
	}

	feed(m, LaunchOK{})
	if m.State() != StateConnecting {
		t.Fatalf("state = %s, want connecting", m.State())
	}

	feed(m, LogEvent{Kind: "queued", Position: 212, Detail: "In queue, position 212"})
	if m.State() != StateQueued || m.Position() != 212 {
		t.Fatalf("state = %s position = %d, want queued/212", m.State(), m.Position())
	}

	feed(m, LogEvent{Kind: "queued", Position: 40})
	if m.Position() != 40 {
		t.Fatalf("position = %d, want 40", m.Position())
	}

	feed(m, LogEvent{Kind: "joined", Detail: "You're in."})
	if m.State() != StateInServer {
		t.Fatalf("state = %s, want in_server", m.State())
	}

	// Not done yet: we hold the slot for a bit before declaring success.
	feed(m, Tick{})
	if m.State() != StateInServer {
		t.Fatalf("job finished too early, before the confirm window elapsed")
	}
	c.advance(31 * time.Second)
	feed(m, Tick{})
	if m.State() != StateDone {
		t.Fatalf("state = %s, want done", m.State())
	}
}

func TestRepeatedSamePositionDoesNotSpamUpdates(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 100})
	res := m.Handle(LogEvent{Kind: "queued", Position: 100})
	if len(res.Transitions) != 0 {
		t.Fatalf("the same queue position produced %d updates; the phone would buzz for nothing", len(res.Transitions))
	}
}

// A Steam that is genuinely signed out cannot be fixed by waiting, so the job
// has to conclude, say so plainly, and not spend its whole retry budget finding
// out. It does get a couple of quick tries first, because a restarting Steam
// looks identical for a few seconds: see
// TestATransientSteamProblemIsRetriedBeforeGivingUp.
func TestASignedOutSteamConcludesQuicklyWithAPlainReason(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 8, RetryBase: time.Second, RetryMax: time.Second})

	feed(m, Start{})
	for i := 0; i < 6 && m.State() != StateFailed; i++ {
		feed(m, LaunchOK{}, LogEvent{Kind: "steam_problem", Detail: "Steam isn't logged in."})
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}

	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed", m.State())
	}
	if m.Failure() == nil || m.Failure().Code != "steam_problem" {
		t.Fatalf("failure = %+v, want a steam_problem reason", m.Failure())
	}
	if m.Attempt() > steamProblemRetries+1 {
		t.Fatalf("attempts = %d, want no more than %d: we must not burn the whole budget on this",
			m.Attempt(), steamProblemRetries+1)
	}
}

func TestRejectionIsNotRetried(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "rejected", Detail: "You are banned from this server."})
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed", m.State())
	}
	if m.Failure().Message != "You are banned from this server." {
		t.Fatalf("the user must be told the real reason, got %q", m.Failure().Message)
	}
}

func TestCrashMidQueueRelaunches(t *testing.T) {
	m, c := newTestMachine(Config{RetryBase: 5 * time.Second})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 240})

	feed(m, GameExited{Code: 1})
	if m.State() != StateRetrying {
		t.Fatalf("state = %s, want retrying after a crash", m.State())
	}
	if m.Position() != 240 {
		t.Log("note: position is retained until the relaunch, which is what the phone shows")
	}

	// Too early: still backing off.
	feed(m, Tick{})
	if m.State() != StateRetrying {
		t.Fatalf("relaunched before the backoff elapsed")
	}
	c.advance(6 * time.Second)
	if acts := actionsFor(m, Tick{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("expected a relaunch after the backoff, got %v", acts)
	}
	if m.Attempt() != 2 {
		t.Fatalf("attempts = %d, want 2", m.Attempt())
	}
	if m.Position() != 0 {
		t.Fatalf("queue position should reset on relaunch, got %d", m.Position())
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	m, _ := newTestMachine(Config{RetryBase: 5 * time.Second, RetryMax: 20 * time.Second})
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 20 * time.Second}
	for i, w := range want {
		m.attempt = i + 1
		if got := m.backoff(); got != w {
			t.Errorf("attempt %d backoff = %s, want %s", i+1, got, w)
		}
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 3, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{})
	for i := 0; i < 10 && m.State() != StateFailed; i++ {
		feed(m, LaunchOK{}, GameExited{Code: 1})
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed after running out of attempts", m.State())
	}
	if m.Failure().Code != "gave_up" {
		t.Fatalf("failure code = %s, want gave_up", m.Failure().Code)
	}
	if m.Attempt() > 3 {
		t.Fatalf("made %d attempts, cap was 3", m.Attempt())
	}
}

// Wipe day: the server bounces up and down before it settles. We must not
// launch on a false start, and we must not give up.
func TestServerFlappingDoesNotBurnAttempts(t *testing.T) {
	m, c := newTestMachine(Config{WaitForServerUp: true, ConnectJitterMax: 4 * time.Second})
	feed(m, Start{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s, want waiting_for_server_up", m.State())
	}

	// Up, then down again before our (jitter/2 = 2s) delay elapses.
	feed(m, ServerUp{})
	c.advance(1 * time.Second)
	feed(m, Tick{})
	feed(m, ServerDown{})
	c.advance(5 * time.Second)
	feed(m, Tick{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s: a false start launched the game", m.State())
	}
	if m.Attempt() != 0 {
		t.Fatalf("attempts = %d, want 0: flapping must not cost us a retry", m.Attempt())
	}

	// Now it comes up for real.
	feed(m, ServerUp{})
	c.advance(3 * time.Second)
	if acts := actionsFor(m, Tick{}); len(acts) != 1 || acts[0] != ActionLaunchGame {
		t.Fatalf("expected a launch once the server settled, got %v", acts)
	}
}

func TestConnectAttemptsAreRateLimited(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, MaxConnectsPerMinute: 2,
		ConnectJitterMax: 0, RetryBase: time.Second, RetryMax: time.Second, MaxAttempts: 20,
	})
	feed(m, Start{})
	launches := 0
	for i := 0; i < 10; i++ {
		feed(m, ServerUp{})
		res := m.Handle(Tick{})
		for _, a := range res.Actions {
			if a == ActionLaunchGame {
				launches++
			}
		}
		if m.State() == StateLaunching {
			feed(m, LaunchOK{}, GameExited{Code: 1}) // instant death, straight back round
		}
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}
	if launches > 2 {
		t.Fatalf("launched %d times inside a minute, cap was 2", launches)
	}
}

func TestCancelFromQueueClosesTheGame(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 5})
	res := m.Handle(Cancel{})
	if m.State() != StateDone {
		t.Fatalf("state = %s, want done", m.State())
	}
	found := false
	for _, a := range res.Actions {
		if a == ActionCloseGame {
			found = true
		}
	}
	if !found {
		t.Fatal("cancelling must close the game, otherwise Rust sits in the queue forever")
	}
}

func TestFinishedJobsIgnoreEverything(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "rejected"})
	if m.State() != StateFailed {
		t.Fatal("setup failed")
	}
	if res := m.Handle(LogEvent{Kind: "joined"}); len(res.Transitions) != 0 {
		t.Fatal("a finished job moved again")
	}
}

// On force wipe the game often cannot start because Steam is still fetching the
// update that ships with the wipe. If Steam then wedges, the player must be told
// that, not told their game crashed: one of those sentences sends them to look
// at Steam, the other sends them nowhere.
func TestAStuckSteamUpdateExplainsItselfRatherThanLookingLikeACrash(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 2, RetryBase: time.Second, RetryMax: time.Second})
	const stuck = "Steam has stopped downloading Rust (1.0 GB of 4.0 GB done). Nothing has moved for 9 minutes."

	feed(m, Start{}, LaunchOK{})
	trs := m.Handle(GameExited{Code: -1, Reason: stuck}).Transitions

	if m.State() != StateRetrying {
		t.Fatalf("state = %s, want retrying", m.State())
	}
	if len(trs) == 0 || !strings.Contains(trs[0].Detail, "stopped downloading") {
		t.Fatalf("the player was not told about Steam: %+v", trs)
	}
	if strings.Contains(trs[0].Detail, "closed unexpectedly") {
		t.Fatalf("a stuck download was reported as a crash: %q", trs[0].Detail)
	}

	// And it still gives up eventually rather than retrying forever.
	c.advance(2 * time.Second)
	feed(m, Tick{}, LaunchOK{})
	m.Handle(GameExited{Code: -1, Reason: stuck})
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed after the attempts ran out", m.State())
	}
}

// A plain crash with nothing known about it still reads as a crash.
func TestAnExitWithNoReasonIsStillACrash(t *testing.T) {
	m, _ := newTestMachine(Config{MaxAttempts: 3})
	feed(m, Start{}, LaunchOK{})
	trs := m.Handle(GameExited{Code: 1}).Transitions
	if len(trs) == 0 || !strings.Contains(trs[0].Detail, "closed unexpectedly") {
		t.Fatalf("expected the crash wording, got %+v", trs)
	}
}

// Steam restarts itself when it updates, and on force wipe that is exactly when
// it happens. Rust launched in that window can fail to reach Steam for a few
// seconds. Failing permanently on the first sighting turns a blip into a missed
// wipe, so it gets a couple of quick retries first.
func TestATransientSteamProblemIsRetriedBeforeGivingUp(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 8, RetryBase: time.Second, RetryMax: time.Second})

	feed(m, Start{}, LaunchOK{})
	feed(m, LogEvent{Kind: "steam_problem", Detail: "Steam isn't logged in."})
	if m.State() != StateFailed {
		// good: it is retrying rather than giving up
	}
	if m.State() == StateFailed {
		t.Fatal("the first Steam problem failed the job outright; a restarting Steam gets no second chance")
	}

	// Second sighting: still worth one more try.
	c.advance(2 * time.Second)
	feed(m, Tick{}, LaunchOK{})
	feed(m, LogEvent{Kind: "steam_problem"})
	if m.State() == StateFailed {
		t.Fatal("gave up on the second Steam problem, sooner than intended")
	}

	// Third: Steam really is logged out. Stop, and say so plainly.
	c.advance(2 * time.Second)
	feed(m, Tick{}, LaunchOK{})
	feed(m, LogEvent{Kind: "steam_problem"})
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want failed: a genuinely logged-out Steam is not fixed by waiting", m.State())
	}
	if f := m.Failure(); f == nil || f.Code != "steam_problem" {
		t.Fatalf("failure = %+v, want the Steam reason so the player knows where to look", f)
	}
}

// It must not spend its whole retry budget on Steam, either.
func TestSteamProblemsStopWellShortOfTheAttemptLimit(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 8, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{})
	for i := 0; i < 8 && m.State() != StateFailed; i++ {
		feed(m, LaunchOK{}, LogEvent{Kind: "steam_problem"})
		c.advance(2 * time.Second)
		feed(m, Tick{})
	}
	if m.State() != StateFailed {
		t.Fatal("never gave up on a logged-out Steam")
	}
	if m.Attempt() > 4 {
		t.Errorf("used %d attempts on a Steam problem; it should conclude quickly", m.Attempt())
	}
}

// The player closes Rust themselves, mid-queue, perhaps to play something else.
// Relaunching the game in their face is not acceptable: their close IS the
// cancel, and the job ends cleanly.
func TestPlayerClosingRustEndsTheJobInsteadOfFightingThem(t *testing.T) {
	m, _ := newTestMachine(Config{MaxAttempts: 8})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 240})

	// The graceful shutdown line arrives, then the process is gone.
	feed(m, LogEvent{Kind: "user_quit"})
	res := m.Handle(GameExited{Code: 0})

	if m.State() != StateDone {
		t.Fatalf("state = %s, want done", m.State())
	}
	for _, a := range res.Actions {
		if a == ActionLaunchGame {
			t.Fatal("it relaunched the game the player just closed")
		}
	}
	if len(res.Transitions) == 0 || res.Transitions[0].Reason == nil ||
		res.Transitions[0].Reason.Code != "player_closed" {
		t.Fatalf("the phone was not told the player closed it: %+v", res.Transitions)
	}
}

// The same farewell lines appear when WE close the game for a retry, and they
// must not be mistaken for the player quitting, or every retry would cancel
// itself.
func TestOurOwnCloseIsNotMistakenForThePlayerQuitting(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 3, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 100})

	// The server drops us: retryOrFail closes the game, and the closing game
	// writes its shutdown lines.
	feed(m, LogEvent{Kind: "disconnected", Detail: "Disconnected: timed out"})
	if m.State() != StateRetrying {
		t.Fatalf("setup: state = %s, want retrying", m.State())
	}
	feed(m, LogEvent{Kind: "user_quit"}) // the farewell from the close WE ordered

	c.advance(2 * time.Second)
	res := m.Handle(Tick{})
	launched := false
	for _, a := range res.Actions {
		if a == ActionLaunchGame {
			launched = true
		}
	}
	if !launched {
		t.Fatalf("the retry never relaunched: our own close was misread as the player quitting (state %s)", m.State())
	}
}

// A crash writes no farewell. It must still be retried exactly as before.
func TestACrashStillRelaunches(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 3, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 100})
	feed(m, GameExited{Code: 1}) // no user_quit line first
	if m.State() != StateRetrying {
		t.Fatalf("state = %s, want retrying after a crash", m.State())
	}
	c.advance(2 * time.Second)
	feed(m, Tick{})
	if m.Attempt() != 2 {
		t.Fatalf("attempts = %d, want 2: the crash was not retried", m.Attempt())
	}
}

// A second launch starts clean: the previous copy's farewell must not bleed
// into the next attempt and cancel it.
func TestUserQuitFlagDoesNotOutliveTheLaunchItBelongsTo(t *testing.T) {
	m, c := newTestMachine(Config{MaxAttempts: 4, RetryBase: time.Second, RetryMax: time.Second})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 50})

	// Round one: a genuine player quit would end it, but here the disconnect
	// arrives first so we are already retrying and ignore the farewell.
	feed(m, LogEvent{Kind: "disconnected"})
	feed(m, LogEvent{Kind: "user_quit"})
	c.advance(2 * time.Second)
	feed(m, Tick{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 45})

	// Round two: the new copy crashes. If the old farewell leaked through, this
	// would be read as the player quitting.
	res := m.Handle(GameExited{Code: 1})
	if m.State() == StateDone {
		t.Fatal("a crash in attempt two was misread as the player quitting, via a stale flag")
	}
	_ = res
}

// Rust does not log queue positions (verified against a real full-server
// session, 2026-08-29: five minutes queued, nothing in the log between
// Connecting and Spawning World). So while the client sits connecting, the
// queue on the phone comes from the SERVER's own answer: how many are in line.
func TestServerReportedQueueDrivesTheQueueDisplay(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{})
	if m.State() != StateConnecting {
		t.Fatalf("setup: state = %s", m.State())
	}

	// The server says 8 people are in line while we are connecting: we are one
	// of them.
	states := feed(m, ServerUp{Players: 200, MaxPlayers: 200, Queue: 8})
	if m.State() != StateQueued || m.Position() != 8 {
		t.Fatalf("state = %s position = %d, want queued/8 (%v)", m.State(), m.Position(), states)
	}

	// The line shrinks; the phone follows.
	feed(m, ServerUp{Queue: 5})
	if m.Position() != 5 {
		t.Fatalf("position = %d, want 5", m.Position())
	}

	// The same number again must not produce another update.
	if res := m.Handle(ServerUp{Queue: 5}); len(res.Transitions) != 0 {
		t.Fatalf("a repeated queue count produced %d updates", len(res.Transitions))
	}

	// Front of the line.
	trs := m.Handle(ServerUp{Queue: 0}).Transitions
	if len(trs) == 0 || !strings.Contains(trs[0].Detail, "front") {
		t.Fatalf("reaching the front was not reported: %+v", trs)
	}

	// And then the real join, from the log.
	feed(m, LogEvent{Kind: "joined", Detail: "You're in the server."})
	if m.State() != StateInServer {
		t.Fatalf("state = %s, want in_server", m.State())
	}
}

// If the game's log DOES report a position (an older build, a changed patch),
// that number is the player's own place and beats the server's count. The
// coarser server number must not overwrite it.
func TestALogReportedPositionOutranksTheServerCount(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, LogEvent{Kind: "queued", Position: 3, Detail: "In queue, position 3"})
	feed(m, ServerUp{Queue: 40})
	if m.Position() != 3 {
		t.Fatalf("position = %d; the server's queue length overwrote the player's own position", m.Position())
	}
}

// A queue reported while we are only WAITING for the server, or after we are
// in, is other people's queue, not ours.
func TestServerQueueOnlyCountsWhileConnecting(t *testing.T) {
	m, _ := newTestMachine(Config{WaitForServerUp: true})
	feed(m, Start{})
	feed(m, ServerUp{Queue: 50})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s; a queue we are not in moved the machine", m.State())
	}

	m2, _ := newTestMachine(Config{})
	feed(m2, Start{}, LaunchOK{}, LogEvent{Kind: "joined"})
	feed(m2, ServerUp{Queue: 12})
	if m2.State() != StateInServer {
		t.Fatalf("state = %s; a queue behind us moved the machine", m2.State())
	}
}

// The number shown is an estimate of the player's place: the lowest queue
// length seen since they joined it. People joining behind them grow the line
// but must never push their number back up.
func TestQueueEstimateOnlyMovesTowardTheFront(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, ServerUp{Queue: 8})
	if m.Position() != 8 {
		t.Fatalf("position = %d, want 8", m.Position())
	}
	// Five more people arrive behind: total 13. The player has not moved back.
	if res := m.Handle(ServerUp{Queue: 13}); len(res.Transitions) != 0 || m.Position() != 8 {
		t.Fatalf("a growing line pushed the player's number up: position %d, %d transitions",
			m.Position(), len(res.Transitions))
	}
	// The front admits three: total 5, and that IS progress.
	feed(m, ServerUp{Queue: 5})
	if m.Position() != 5 {
		t.Fatalf("position = %d, want 5", m.Position())
	}
}

// The real end of a queue, as the real log shows it: 'Loading World' arrives,
// the queue display retires, and the counts of the people still waiting behind
// must not drag the display back into 'queued'.
func TestLoadingEndsTheQueueDisplayForGood(t *testing.T) {
	m, _ := newTestMachine(Config{})
	feed(m, Start{}, LaunchOK{}, ServerUp{Queue: 8})
	if m.State() != StateQueued {
		t.Fatalf("setup: state = %s", m.State())
	}

	trs := m.Handle(LogEvent{Kind: "loading", Detail: "Through the queue. Loading into the server."}).Transitions
	if m.State() != StateConnecting {
		t.Fatalf("state = %s, want connecting once loading starts", m.State())
	}
	if len(trs) == 0 || !strings.Contains(trs[0].Detail, "Through the queue") {
		t.Fatalf("the phone was not told the queue is over: %+v", trs)
	}

	// Other people are still queued. That is their problem, not this display's.
	feed(m, ServerUp{Queue: 42})
	if m.State() != StateQueued && m.State() == StateConnecting {
		// still connecting: correct
	} else {
		t.Fatalf("state = %s; someone else's queue dragged the display back", m.State())
	}

	feed(m, LogEvent{Kind: "joined", Detail: "You're in the server."})
	if m.State() != StateInServer {
		t.Fatalf("state = %s, want in_server", m.State())
	}
}
