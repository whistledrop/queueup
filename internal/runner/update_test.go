package runner_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/runner"
	"queueup/internal/scenario"
	"queueup/internal/serverstat"
)

// updatingLauncher wraps the simulator and reports a Steam download in
// progress, the way force wipe day looks: the scheduled join has fired, the
// server is coming back, and Steam still has gigabytes to fetch.
type updatingLauncher struct {
	*game.SimLauncher

	mu    sync.Mutex
	state game.UpdateState
}

func (u *updatingLauncher) UpdateProgress() game.UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

func (u *updatingLauncher) setState(s game.UpdateState) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state = s
}

// The force-wipe schedule story, end to end through the runner: the job starts
// while Steam is mid-download, the phone hears about the download and its
// progress while the job is still WAITING for the server, and once the update
// finishes the join proceeds to done. This is the exact combination a
// scheduled join meets at 7pm on the first Thursday of the month.
func TestScheduledJoinWaitsThroughASteamUpdateAndStillGetsIn(t *testing.T) {
	sc, err := scenario.Load("../../testdata/scenarios/instant_join.json")
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}

	launcher := &updatingLauncher{
		SimLauncher: &game.SimLauncher{
			Scenario: sc, Log: filepath.Join(t.TempDir(), "Player.log"), Speed: 20,
		},
	}
	launcher.setState(game.UpdateState{
		Known: true, Updating: true,
		BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30,
	})

	// The server is down (wipe restart in progress) and comes back shortly.
	server := serverstat.NewScripted([]scenario.ServerPoint{
		{AfterMs: 0, Online: false},
		{AfterMs: 8000, Online: true, Players: 10, Max: 200},
	}, 20)

	m := job.New(job.Config{
		WaitForServerUp:  true,
		InServerConfirm:  300 * time.Millisecond,
		ConnectJitterMax: 50 * time.Millisecond,
	})

	var mu sync.Mutex
	var notes []string
	r := &runner.Runner{
		Machine: m, Launcher: launcher, Parser: parser, Server: server,
		Addr: game.Addr{IP: "51.83.128.10", Port: 28015},
		Tick: 50 * time.Millisecond, ServerPoll: 50 * time.Millisecond, LogPoll: 20 * time.Millisecond,
		OnNote: func(n string) {
			mu.Lock()
			notes = append(notes, n)
			mu.Unlock()
		},
	}

	// The download finishes shortly after the job starts, as it would once
	// Steam catches up.
	go func() {
		time.Sleep(600 * time.Millisecond)
		launcher.setState(game.UpdateState{Known: true, Installed: true})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	final := r.Run(ctx)
	_ = launcher.Close()

	if final != job.StateDone {
		t.Fatalf("final = %s, want done: the update stopped the scheduled join", final)
	}

	mu.Lock()
	defer mu.Unlock()
	var told bool
	for _, n := range notes {
		if strings.Contains(n, "Steam is updating Rust") {
			told = true
		}
	}
	if !told {
		t.Fatalf("the phone was never told about the download while waiting; notes: %v", notes)
	}
}

// A stalled download during a scheduled join must say so, not sit quiet.
func TestScheduledJoinReportsAStalledDownload(t *testing.T) {
	sc, err := scenario.Load("../../testdata/scenarios/instant_join.json")
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	launcher := &updatingLauncher{
		SimLauncher: &game.SimLauncher{
			Scenario: sc, Log: filepath.Join(t.TempDir(), "Player.log"), Speed: 20,
		},
	}
	launcher.setState(game.UpdateState{
		Known: true, Updating: true, Paused: true,
		BytesToDownload: 4 << 30, BytesDownloaded: 1 << 30,
	})

	m := job.New(job.Config{WaitForServerUp: true})
	var mu sync.Mutex
	var notes []string
	r := &runner.Runner{
		Machine: m, Launcher: launcher, Parser: parser,
		Server: serverstat.NewScripted([]scenario.ServerPoint{{AfterMs: 0, Online: false}}, 1),
		Addr:   game.Addr{IP: "51.83.128.10", Port: 28015},
		Tick:   50 * time.Millisecond, ServerPoll: 50 * time.Millisecond, LogPoll: 20 * time.Millisecond,
		OnNote: func(n string) { mu.Lock(); notes = append(notes, n); mu.Unlock() },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.Run(ctx)
	_ = launcher.Close()

	mu.Lock()
	defer mu.Unlock()
	for _, n := range notes {
		if strings.Contains(n, "paused") && strings.Contains(n, "Steam") {
			return
		}
	}
	t.Fatalf("a paused download during a scheduled join was never reported; notes: %v", notes)
}
