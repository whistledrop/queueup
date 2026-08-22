package logtail_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"queueup/internal/logtail"
)

type collector struct {
	mu    sync.Mutex
	lines []string
}

func (c *collector) add(l string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, l)
}

func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func (c *collector) waitFor(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.snapshot(); len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lines, got %v", n, c.snapshot())
	return nil
}

// The file often does not exist when the agent starts watching. That must not
// be an error, and lines written later must still be picked up.
func TestFollowWaitsForAFileThatDoesNotExistYet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Player.log")

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 20*time.Millisecond, true, c.add)

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(path, []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := c.waitFor(t, 2)
	if got[0] != "first line" || got[1] != "second line" {
		t.Fatalf("lines = %v", got)
	}
}

// Rust truncates Player.log on every launch. The tailer has to notice and read
// the new session from the top, or a relaunch would look silent.
func TestFollowRereadsAfterTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Player.log")
	if err := os.WriteFile(path, []byte("session one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 20*time.Millisecond, true, c.add)
	c.waitFor(t, 1)

	// Relaunch: truncate and write a shorter new session.
	if err := os.WriteFile(path, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := c.waitFor(t, 2)
	if got[len(got)-1] != "two" {
		t.Fatalf("did not pick up the new session after truncation, lines = %v", got)
	}
}

// Half-written lines must not be delivered until their newline arrives.
func TestFollowDoesNotEmitPartialLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Player.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 20*time.Millisecond, true, c.add)

	if _, err := f.WriteString("You are in queue posi"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := c.snapshot(); len(got) != 0 {
		t.Fatalf("emitted a partial line: %v", got)
	}
	if _, err := f.WriteString("tion 212\n"); err != nil {
		t.Fatal(err)
	}
	got := c.waitFor(t, 1)
	if got[0] != "You are in queue position 212" {
		t.Fatalf("line = %q", got[0])
	}
}

// The agent starts watching a Player.log that still holds the PREVIOUS Rust
// session. Reading it would make the agent act on something that happened
// yesterday: a stale "you are banned" line would fail a job that never started.
// So old content is skipped, but the new session, which begins when the game
// truncates the file, must be read in full from the top.
func TestFollowSkipsTheOldSessionButReadsTheNewOneFromTheTop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Player.log")
	old := "Disconnected: You are banned from this server\nsome other old line\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 20*time.Millisecond, false, c.add)

	time.Sleep(150 * time.Millisecond)
	if got := c.snapshot(); len(got) != 0 {
		t.Fatalf("read stale content from the previous session: %v", got)
	}

	// The game launches and truncates the log.
	if err := os.WriteFile(path, []byte("Connecting to 1.2.3.4:28015\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := c.waitFor(t, 1)
	if got[0] != "Connecting to 1.2.3.4:28015" {
		t.Fatalf("first line of the new session = %q, want the connect line", got[0])
	}
}

// When the log does not exist yet (a fresh PC, or the game has never been run
// since the last cleanup), the game creates it and immediately writes to it. We
// must not miss those first lines: on a fast failure, "Steam isn't logged in" is
// written and the process is gone milliseconds later, and that line is the whole
// point of the job.
func TestFollowReadsFromTheTopWhenTheFileAppearsAfterWatchingBegins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Player.log")

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 20*time.Millisecond, false, c.add)

	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(path, []byte("Steamworks failed to initialize\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := c.waitFor(t, 1)
	if got[0] != "Steamworks failed to initialize" {
		t.Fatalf("line = %q, want the Steam failure line", got[0])
	}
}
