package runner_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/runner"
	"queueup/internal/scenario"
	"queueup/internal/serverstat"
)

// These are the real end-to-end tests: the actual agent code, the actual log
// tailer, the actual parser and the actual state machine, running against the
// fake Rust client. No game, no Steam, no Windows, and they finish in seconds.

const speed = 20 // compress every scenario timeline 20x

func runScenario(t *testing.T, name string, cfg job.Config) (job.State, *job.Machine, []job.Transition) {
	t.Helper()

	sc, err := scenario.Load(filepath.Join("../../testdata/scenarios", name+".json"))
	if err != nil {
		t.Fatalf("loading scenario: %v", err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatalf("loading patterns: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "Player.log")
	launcher := &game.SimLauncher{Scenario: sc, Log: logPath, Speed: speed}

	var server serverstat.Source = serverstat.AlwaysUp{}
	if len(sc.Server) > 0 {
		server = serverstat.NewScripted(sc.Server, speed)
		cfg.WaitForServerUp = true
	}

	m := job.New(cfg)
	var seen []job.Transition
	r := &runner.Runner{
		Machine:      m,
		Launcher:     launcher,
		Parser:       parser,
		Server:       server,
		Addr:         game.Addr{IP: "51.83.128.10", Port: 28015},
		Tick:         50 * time.Millisecond,
		ServerPoll:   50 * time.Millisecond,
		LogPoll:      20 * time.Millisecond,
		OnTransition: func(tr job.Transition) { seen = append(seen, tr) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	final := r.Run(ctx)
	_ = launcher.Close()
	return final, m, seen
}

func statesOf(trs []job.Transition) []job.State {
	var out []job.State
	for _, t := range trs {
		out = append(out, t.To)
	}
	return out
}

func contains(states []job.State, want job.State) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}

func TestInstantJoin(t *testing.T) {
	final, _, trs := runScenario(t, "instant_join", job.Config{InServerConfirm: 300 * time.Millisecond})
	if final != job.StateDone {
		t.Fatalf("final = %s, want done. transitions: %v", final, statesOf(trs))
	}
	if !contains(statesOf(trs), job.StateInServer) {
		t.Errorf("never reported in_server. transitions: %v", statesOf(trs))
	}
}

func TestLongQueueReportsEveryPosition(t *testing.T) {
	final, _, trs := runScenario(t, "long_queue", job.Config{InServerConfirm: 300 * time.Millisecond})
	if final != job.StateDone {
		t.Fatalf("final = %s, want done. transitions: %v", final, statesOf(trs))
	}
	var positions []int
	for _, tr := range trs {
		if tr.To == job.StateQueued {
			positions = append(positions, tr.Position)
		}
	}
	want := []int{212, 148, 61, 12, 1}
	if len(positions) != len(want) {
		t.Fatalf("queue positions reported = %v, want %v", positions, want)
	}
	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf("queue positions reported = %v, want %v", positions, want)
		}
	}
}

func TestRejectionStopsImmediately(t *testing.T) {
	final, m, trs := runScenario(t, "rejected", job.Config{MaxAttempts: 5})
	if final != job.StateFailed {
		t.Fatalf("final = %s, want failed. transitions: %v", final, statesOf(trs))
	}
	if m.Attempt() != 1 {
		t.Errorf("made %d attempts on a refusal, want 1", m.Attempt())
	}
	if m.Failure() == nil || m.Failure().Message == "" {
		t.Error("failed with no message for the user")
	}
}

// A signed-out Steam has to end the job with the real reason, and get there
// quickly, without spending the whole retry budget. It does try a couple of
// times first: Steam restarts itself when it updates, which on force wipe day
// is exactly when it happens, and for those few seconds a perfectly healthy
// setup is indistinguishable from a signed-out one.
func TestSteamNotLoggedInIsExplainedAndConcludesQuickly(t *testing.T) {
	final, m, _ := runScenario(t, "steam_not_logged_in", job.Config{
		MaxAttempts: 8, RetryBase: 200 * time.Millisecond, RetryMax: 200 * time.Millisecond,
	})
	if final != job.StateFailed {
		t.Fatalf("final = %s, want failed", final)
	}
	if m.Failure().Code != "steam_problem" {
		t.Fatalf("reason = %+v, want steam_problem", m.Failure())
	}
	if m.Attempt() > 3 {
		t.Errorf("made %d attempts on a signed-out Steam, want no more than 3", m.Attempt())
	}
}

func TestCrashMidQueueRelaunchesUntilItGivesUp(t *testing.T) {
	// The scenario crashes every single time, so the correct behaviour is:
	// relaunch, crash, relaunch, then stop and say so.
	final, m, trs := runScenario(t, "crash_mid_queue", job.Config{
		MaxAttempts: 2, RetryBase: 200 * time.Millisecond, RetryMax: 200 * time.Millisecond,
	})
	if final != job.StateFailed {
		t.Fatalf("final = %s, want failed. transitions: %v", final, statesOf(trs))
	}
	if m.Attempt() != 2 {
		t.Fatalf("attempts = %d, want 2 (it must relaunch after a crash)", m.Attempt())
	}
	if !contains(statesOf(trs), job.StateRetrying) {
		t.Errorf("never entered retrying. transitions: %v", statesOf(trs))
	}
	if m.Failure().Code != "gave_up" {
		t.Errorf("reason = %+v, want gave_up", m.Failure())
	}
}

func TestServerFullBacksOff(t *testing.T) {
	final, m, trs := runScenario(t, "server_full", job.Config{
		MaxAttempts: 2, RetryBase: 200 * time.Millisecond, RetryMax: 200 * time.Millisecond,
	})
	if final != job.StateFailed {
		t.Fatalf("final = %s, want failed. transitions: %v", final, statesOf(trs))
	}
	if !contains(statesOf(trs), job.StateRetrying) {
		t.Errorf("a full server should be retried, not given up on straight away: %v", statesOf(trs))
	}
	_ = m
}

func TestWipeRestartLoopGetsInWithoutBurningRetries(t *testing.T) {
	final, m, trs := runScenario(t, "wipe_restart_loop", job.Config{
		InServerConfirm: 300 * time.Millisecond,
		// A tight jitter so the flapping actually gets exercised at 20x speed.
		ConnectJitterMax: 50 * time.Millisecond,
	})
	if final != job.StateDone {
		t.Fatalf("final = %s, want done. transitions: %v", final, statesOf(trs))
	}
	if !contains(statesOf(trs), job.StateWaitingForServerUp) {
		t.Errorf("never waited for the server. transitions: %v", statesOf(trs))
	}
	if m.Attempt() > 3 {
		t.Errorf("used %d attempts getting through a restart loop; flapping is eating our retries", m.Attempt())
	}
}

func TestCancelStopsTheJob(t *testing.T) {
	sc, err := scenario.Load("../../testdata/scenarios/long_queue.json")
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	launcher := &game.SimLauncher{Scenario: sc, Log: filepath.Join(t.TempDir(), "Player.log"), Speed: speed}
	m := job.New(job.Config{})
	r := &runner.Runner{
		Machine: m, Launcher: launcher, Parser: parser, Server: serverstat.AlwaysUp{},
		Addr: game.Addr{IP: "51.83.128.10", Port: 28015},
		Tick: 50 * time.Millisecond, ServerPoll: 50 * time.Millisecond, LogPoll: 20 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		// Let it get into the queue, then cancel like the phone would.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if m.State() == job.StateQueued {
				r.Cancel("You cancelled the join.")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	final := r.Run(ctx)
	if final != job.StateDone {
		t.Fatalf("final = %s, want done after a cancel", final)
	}
	time.Sleep(200 * time.Millisecond)
	if launcher.Running() {
		t.Error("the fake game is still running after a cancel")
	}
}

// The entire product is "your PC holds the slot". So when a join SUCCEEDS, the
// game must be left running. Closing it after done would kick the player out of
// the server the moment we finished congratulating them.
func TestAWonSlotIsNotThrownAwayWhenTheJobFinishes(t *testing.T) {
	sc, err := scenario.Load("../../testdata/scenarios/instant_join.json")
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	launcher := &game.SimLauncher{Scenario: sc, Log: filepath.Join(t.TempDir(), "Player.log"), Speed: speed}
	m := job.New(job.Config{InServerConfirm: 300 * time.Millisecond})
	r := &runner.Runner{
		Machine: m, Launcher: launcher, Parser: parser, Server: serverstat.AlwaysUp{},
		Addr: game.Addr{IP: "51.83.128.10", Port: 28015},
		Tick: 50 * time.Millisecond, ServerPoll: 50 * time.Millisecond, LogPoll: 20 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if final := r.Run(ctx); final != job.StateDone {
		t.Fatalf("final = %s, want done", final)
	}
	// Give any close that should not happen a moment to happen.
	time.Sleep(300 * time.Millisecond)
	if !launcher.Running() {
		t.Fatal("the game was closed after a successful join; the slot we just won is gone")
	}
	_ = launcher.Close() // tidy up the fake game now the assertion is made
}

// The full path for the player closing Rust themselves: the fake game writes
// its farewell and exits without being asked, and the job must end cleanly with
// one attempt made and the game left closed.
func TestPlayerQuitMidQueueEndsCleanlyWithoutARelaunch(t *testing.T) {
	final, m, trs := runScenario(t, "user_quit_mid_queue", job.Config{
		MaxAttempts: 5, RetryBase: 200 * time.Millisecond, RetryMax: 200 * time.Millisecond,
	})
	if final != job.StateDone {
		t.Fatalf("final = %s, want done. transitions: %v", final, statesOf(trs))
	}
	if m.Attempt() != 1 {
		t.Fatalf("made %d attempts; the game the player closed was relaunched", m.Attempt())
	}
	last := trs[len(trs)-1]
	if last.Reason == nil || last.Reason.Code != "player_closed" {
		t.Fatalf("the ending does not say the player closed it: %+v", last)
	}
	if !strings.Contains(last.Detail, "closed on your PC") {
		t.Errorf("detail %q does not explain what happened in plain words", last.Detail)
	}
}

// The real queue shape, learned from a genuine full server: the log stays
// silent from Connecting until Spawning World, and every queue number on the
// phone comes from polling the server. The counts must flow through, in order,
// while the log says nothing.
func TestQueueNumbersComeFromTheServerWhenTheLogIsSilent(t *testing.T) {
	final, m, trs := runScenario(t, "real_queue_via_server", job.Config{
		InServerConfirm:  300 * time.Millisecond,
		ConnectJitterMax: 50 * time.Millisecond,
	})
	if final != job.StateDone {
		t.Fatalf("final = %s, want done. transitions: %v", final, statesOf(trs))
	}
	var counts []int
	for _, tr := range trs {
		if tr.To == job.StateQueued {
			counts = append(counts, tr.Position)
		}
	}
	if len(counts) < 2 {
		t.Fatalf("queue counts reported = %v; the server's numbers never reached the phone", counts)
	}
	for i := 1; i < len(counts); i++ {
		if counts[i] > counts[i-1] {
			t.Fatalf("queue counts went up: %v", counts)
		}
	}
	if counts[0] != 8 {
		t.Errorf("first count = %d, want the server's 8", counts[0])
	}
	if m.Attempt() != 1 {
		t.Errorf("attempts = %d; queueing must not look like failure", m.Attempt())
	}
}
