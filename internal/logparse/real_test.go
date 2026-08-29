package logparse

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// These lines are REAL, captured from a real PC's output_log.txt on 2026-08-29:
// launch, connect, spawn in, quit. Every assertion here is against something the
// actual game wrote, which makes this the ground truth the guessed patterns
// never had.
func TestRealSessionLinesParseCorrectly(t *testing.T) {
	p := loadTestParser(t)

	expected := map[string]struct {
		kind    EventKind
		address string
	}{
		"Connecting: 168.100.161.129:28189 (Raknet)": {kind: EventConnecting, address: "168.100.161.129:28189"},
		"[23.8s] Spawning World":                     {kind: EventJoined},
		"Steam Client Shutdown":                      {kind: EventUserQuit},
	}

	for fragment, want := range expected {
		line := "2026-08-29T16:01:40.361Z|0x7acc|" + fragment
		ev, ok := p.ParseLine(line)
		if !ok {
			t.Errorf("real line %q was not recognised at all", line)
			continue
		}
		if ev.Kind != want.kind {
			t.Errorf("real line %q parsed as %s, want %s", line, ev.Kind, want.kind)
		}
		if want.address != "" && !strings.Contains(ev.Detail, want.address) {
			t.Errorf("real line %q: detail %q does not carry the address", line, ev.Detail)
		}
	}

	// The quit handler's stack frame, exactly as the real log shows it.
	ev, ok := p.ParseLine("Client:OnApplicationQuit()")
	if !ok || ev.Kind != EventUserQuit {
		t.Errorf("the real quit frame parsed as %v (matched=%v), want user_quit", ev.Kind, ok)
	}
}

// Every line in the real fixtures that is NOT one of the known events must
// parse as nothing. This is the false-positive guard: a session's worth of
// bootstrap chatter, stack traces and oddities like 'Failed to parse favorite
// server endpoint' must never move the state machine.
func TestRealSessionNoiseMatchesNothing(t *testing.T) {
	p := loadTestParser(t)

	meaningful := func(line string) bool {
		for _, frag := range []string{
			"Connecting: ", "Spawning World", "Steam Client Shutdown", "Client:OnApplicationQuit()",
		} {
			if strings.Contains(line, frag) {
				return true
			}
		}
		return false
	}

	for _, fixture := range []string{
		"../../testdata/fixtures/real-session-keylines.txt",
		"../../testdata/fixtures/real-quit-tail.txt",
	} {
		f, err := os.Open(fixture)
		if err != nil {
			t.Fatalf("fixture missing: %v", err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || meaningful(line) {
				continue
			}
			if ev, ok := p.ParseLine(line); ok {
				t.Errorf("noise line %q was misread as %s (pattern %q)", line, ev.Kind, ev.PatternID)
			}
		}
		f.Close()
	}
}
