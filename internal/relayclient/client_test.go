package relayclient

import (
	"testing"
	"time"
)

func TestSocketURL(t *testing.T) {
	cases := map[string]string{
		"https://relay.example.com":  "wss://relay.example.com/agent",
		"https://relay.example.com/": "wss://relay.example.com/agent",
		"http://127.0.0.1:8080":      "ws://127.0.0.1:8080/agent",
	}
	for in, want := range cases {
		got, err := SocketURL(in)
		if err != nil || got != want {
			t.Errorf("SocketURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"relay.example.com", "ftp://relay", ""} {
		if _, err := SocketURL(bad); err == nil {
			t.Errorf("SocketURL(%q) should have failed", bad)
		}
	}
}

// Backoff has to grow (so a relay that is down is not hammered), cap (so a PC is
// never idle for minutes on wipe day), and vary (so every agent in the world
// does not reconnect at the same instant when the relay comes back).
func TestBackoffGrowsCapsAndVaries(t *testing.T) {
	const max = 30 * time.Second

	var last time.Duration
	for attempt := 1; attempt <= 6; attempt++ {
		d := Backoff(attempt, max)
		if d < last {
			t.Errorf("attempt %d waited %s, less than the previous %s", attempt, d, last)
		}
		if d > max+max/4 {
			t.Errorf("attempt %d waited %s, over the cap", attempt, d)
		}
		last = d
	}

	for attempt := 10; attempt < 14; attempt++ {
		if d := Backoff(attempt, max); d < max {
			t.Errorf("attempt %d waited %s, want at least the cap %s", attempt, d, max)
		}
	}

	seen := map[time.Duration]bool{}
	for i := 0; i < 30; i++ {
		seen[Backoff(5, max)] = true
	}
	if len(seen) < 5 {
		t.Errorf("only %d distinct delays across 30 tries; the jitter is not working", len(seen))
	}
}

func TestFirstRetryIsQuick(t *testing.T) {
	if d := Backoff(1, 30*time.Second); d > 2*time.Second {
		t.Errorf("first retry waits %s; a brief blip should recover almost immediately", d)
	}
}
