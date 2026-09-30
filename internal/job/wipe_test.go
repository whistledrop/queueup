package job

import (
	"strings"
	"testing"
	"time"
)

// The wipe-day bug, as a test.
//
// Somebody schedules for 18:50 because the wipe is "about 7". At 18:50 the old
// server is still up and still full. Joining it is the wrong answer: thirty
// seconds later the job is done, the wipe restart throws them out of a job
// that has already finished, nothing rejoins, and the phone goes on saying
// "you're in" until they walk through the door.
func TestWipeModeDoesNotJoinTheServerThatHasNotWipedYet(t *testing.T) {
	m, c := newTestMachine(Config{WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour})

	feed(m, Start{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s", m.State())
	}

	// The old server, up and busy, for ten minutes. Not one launch.
	for i := 0; i < 20; i++ {
		if acts := actionsFor(m, ServerUp{Players: 250, MaxPlayers: 250, Queue: 80}); len(acts) != 0 {
			t.Fatalf("launched into the pre-wipe server: %v", acts)
		}
		c.advance(30 * time.Second)
		if acts := actionsFor(m, Tick{}); len(acts) != 0 {
			t.Fatalf("a tick launched into the pre-wipe server: %v", acts)
		}
	}
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("stopped waiting: %s", m.State())
	}
	if d := m.Snapshot().Detail; !strings.Contains(d, "still up") {
		t.Errorf("the phone does not explain the wait: %q", d)
	}

	// 19:00. The server goes down to wipe.
	feed(m, ServerDown{})
	if d := m.Snapshot().Detail; !strings.Contains(d, "gone down for the wipe") {
		t.Errorf("the phone does not say the wipe started: %q", d)
	}

	// And comes back. NOW we go.
	feed(m, ServerUp{Players: 0, MaxPlayers: 250})
	c.advance(time.Minute)
	if acts := actionsFor(m, Tick{}); len(acts) == 0 {
		t.Fatal("did not launch once the server came back")
	}
	if m.State() != StateLaunching {
		t.Fatalf("state after the restart = %s", m.State())
	}
}

// Somebody who schedules for after the wipe has already happened looks exactly
// like somebody waiting for a wipe that is running late. Waiting all night is
// the worse of the two answers, so eventually we join and say why.
func TestWipeModeGivesUpWaitingEventually(t *testing.T) {
	m, c := newTestMachine(Config{WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour})
	feed(m, Start{})
	feed(m, ServerUp{Players: 120, MaxPlayers: 250})

	c.advance(90 * time.Minute)
	if acts := actionsFor(m, Tick{}); len(acts) != 0 {
		t.Fatal("gave up before the fallback was due")
	}

	c.advance(45 * time.Minute)
	var said string
	for _, tr := range m.Handle(Tick{}).Transitions {
		if strings.Contains(tr.Detail, "No wipe restart") {
			said = tr.Detail
		}
	}
	if said == "" {
		t.Error("the timeline does not say why it joined anyway")
	}
	c.advance(time.Minute)
	if acts := actionsFor(m, Tick{}); len(acts) == 0 {
		t.Fatal("never gave up waiting")
	}
}

// The other side of the same story: they got into the old server somehow, and
// the wipe kicks them out. Rust has to be closed so Steam can finally patch,
// and then they go back in to the NEW server. This is the path that decides
// whether somebody walks in at 8pm to a game or to a desktop.
func TestBeingKickedByTheWipeCarriesThemIntoTheNewServer(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour,
		RetryBase: 5 * time.Second, RetryMax: 5 * time.Second, MaxAttempts: 8,
	})

	// Straight into the server: the fallback path, or a wipe we already caught.
	feed(m, Start{})
	feed(m, ServerDown{}, ServerUp{})
	c.advance(time.Minute)
	feed(m, Tick{}, LaunchOK{}, LogEvent{Kind: "connecting"}, LogEvent{Kind: "joined"})
	if m.State() != StateInServer {
		t.Fatalf("not in the server: %s", m.State())
	}

	// The wipe restart drops them.
	res := m.Handle(LogEvent{Kind: "disconnected"})
	if !hasAction(res.Actions, ActionCloseGame) {
		t.Fatal("Rust was not closed, so Steam can never apply the wipe patch")
	}
	if m.State() != StateRetrying {
		t.Fatalf("state after the drop = %s", m.State())
	}

	// The server is down for its restart while Steam patches.
	feed(m, ServerDown{})
	c.advance(10 * time.Second)
	feed(m, Tick{})
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("did not go back to waiting: %s", m.State())
	}

	// A long Steam update. Nothing gives up.
	for i := 0; i < 40; i++ {
		c.advance(30 * time.Second)
		feed(m, Tick{})
	}
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("gave up during the Steam update: %s", m.State())
	}

	// The new server answers, and we go back in without waiting for a second
	// restart that is never coming.
	feed(m, ServerUp{Players: 3, MaxPlayers: 250})
	c.advance(time.Minute)
	if acts := actionsFor(m, Tick{}); !hasAction(acts, ActionLaunchGame) {
		t.Fatalf("did not relaunch into the new server: %v", acts)
	}
}

// Without wipe mode, nothing changes: join now means join now.
func TestPlainJoinStillConnectsStraightAway(t *testing.T) {
	m, _ := newTestMachine(Config{})
	if acts := actionsFor(m, Start{}); !hasAction(acts, ActionLaunchGame) {
		t.Fatalf("a plain join did not launch: %v", acts)
	}
}

func hasAction(acts []Action, want Action) bool {
	for _, a := range acts {
		if a == want {
			return true
		}
	}
	return false
}

// Scheduling for AFTER the wipe has already happened.
//
// This is the case "wait for the restart" cannot see on its own: the server is
// up and is never going down again tonight, so waiting for a restart would cost
// the whole evening. Rust says when it started, so we can simply look.
func TestAFreshlyWipedServerIsJoinedWithoutWaiting(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour,
		FreshServerAge: 30 * time.Minute,
	})
	feed(m, Start{})

	// The wipe was twenty minutes ago; this is the new server.
	var said string
	for _, tr := range m.Handle(ServerUp{Players: 40, MaxPlayers: 250, Age: 20 * time.Minute}).Transitions {
		if strings.Contains(tr.Detail, "already wiped") {
			said = tr.Detail
		}
	}
	if said == "" {
		t.Error("the timeline does not explain why it did not wait")
	}
	c.advance(time.Minute)
	if acts := actionsFor(m, Tick{}); !hasAction(acts, ActionLaunchGame) {
		t.Fatalf("did not join a server that had already wiped: %v", acts)
	}
	if !strings.Contains(said, "20 minutes") {
		t.Errorf("the age is not in plain words: %q", said)
	}
}

// The other side of the same coin: a server that has been up for hours has not
// wiped yet, whatever the clock says, so we still wait.
func TestAnOldServerIsStillWaitedOutEvenWithAnAge(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour,
		FreshServerAge: 30 * time.Minute,
	})
	feed(m, Start{})
	for i := 0; i < 10; i++ {
		if acts := actionsFor(m, ServerUp{Players: 250, MaxPlayers: 250, Age: 9 * time.Hour}); len(acts) != 0 {
			t.Fatalf("joined a server that has been up nine hours: %v", acts)
		}
		c.advance(time.Minute)
		if acts := actionsFor(m, Tick{}); len(acts) != 0 {
			t.Fatalf("a tick joined the pre-wipe server: %v", acts)
		}
	}
}

// A server that does not publish its age teaches us nothing, so we fall back to
// the old, safe behaviour of waiting for the restart. Unknown must never be
// read as "brand new", or every wipe job would join the server it is supposed
// to be waiting on.
func TestAnUnknownAgeIsNotTreatedAsFresh(t *testing.T) {
	m, c := newTestMachine(Config{
		WaitForServerUp: true, WipeWaitFallback: 2 * time.Hour,
		FreshServerAge: 30 * time.Minute,
	})
	feed(m, Start{})
	if acts := actionsFor(m, ServerUp{Players: 250, MaxPlayers: 250}); len(acts) != 0 {
		t.Fatalf("an unknown age was treated as freshly wiped: %v", acts)
	}
	c.advance(time.Minute)
	if acts := actionsFor(m, Tick{}); len(acts) != 0 {
		t.Fatalf("an unknown age was treated as freshly wiped: %v", acts)
	}
	if m.State() != StateWaitingForServerUp {
		t.Fatalf("state = %s", m.State())
	}
}
