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

// What happened on a real PC on 2026-10-06. The previous session had left a
// long log, Rust relaunched and started a fresh one in the same file, and by
// the next look the new session was already longer than the old one had been.
// Nothing ever looked shorter, so the tailer carried on from the old session's
// end, in the middle of the new one, and missed "Connecting", the map and
// "Spawning World" entirely: QueueUp sat on "connecting" for 39 minutes while
// the player was in the server, then read them closing Rust as a crash and
// relaunched it.
//
// On Windows it is worse, because the size Windows reports for a file another
// program is writing can lag behind. So a new session is recognised by its
// first bytes changing, not only by the file shrinking.
func TestFollowNoticesANewSessionThatOutgrewTheOldOneBeforeTheNextLook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output_log.txt")
	old := "2026-10-05T20:00:00.000Z|0x1|old session starts\n"
	for len(old) < 2000 {
		old += "2026-10-05T20:00:01.000Z|0x1|old chatter\n"
	}
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &collector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logtail.Follow(ctx, path, 150*time.Millisecond, false, c.add)
	time.Sleep(400 * time.Millisecond) // the old session has been skipped

	// The relaunch: the same file emptied and refilled, past its old length,
	// all between two looks.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	fresh := "2026-10-06T14:05:29.302Z|0x2|new session starts\n" +
		"2026-10-06T14:05:50.961Z|0x2|Connecting: 185.189.255.232:28015 (Raknet)\n"
	for len(fresh) < 3000 {
		fresh += "2026-10-06T14:06:00.000Z|0x2|new chatter\n"
	}
	fresh += "2026-10-06T14:07:29.408Z|0x2|[49.4s] Spawning World\n"
	if _, err := f.WriteString(fresh); err != nil {
		t.Fatal(err)
	}
	f.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := c.snapshot()
		var connect, spawn bool
		for _, l := range got {
			connect = connect || l == "2026-10-06T14:05:50.961Z|0x2|Connecting: 185.189.255.232:28015 (Raknet)"
			spawn = spawn || l == "2026-10-06T14:07:29.408Z|0x2|[49.4s] Spawning World"
		}
		if connect && spawn {
			for _, l := range got {
				if l == "2026-10-05T20:00:01.000Z|0x1|old chatter" {
					t.Fatal("read the previous session's lines")
				}
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missed the new session's lines; got %d lines, first %q", len(c.snapshot()), first(c.snapshot()))
}

func first(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}
