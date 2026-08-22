package logparse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const patternFilePath = "../../configs/patterns.json"

func loadTestParser(t *testing.T) *Parser {
	t.Helper()
	p, err := Load(patternFilePath)
	if err != nil {
		t.Fatalf("loading patterns: %v", err)
	}
	return p
}

// The pattern file must always compile and be internally consistent. This is
// the guard rail for the "edit the JSON when Rust patches" workflow: a typo
// there fails the test suite instead of silently breaking wipe day.
func TestPatternFileIsValid(t *testing.T) {
	loadTestParser(t)
}

// Every pattern carries an example line. That example must be matched by that
// pattern and no earlier one, which keeps the ordering honest.
func TestEachExampleMatchesItsOwnPattern(t *testing.T) {
	p := loadTestParser(t)
	for _, pat := range p.patterns {
		if pat.Example == "" {
			t.Errorf("pattern %q has no example line", pat.ID)
			continue
		}
		ev, ok := p.ParseLine(pat.Example)
		if !ok {
			t.Errorf("pattern %q: its own example %q parses to nothing", pat.ID, pat.Example)
			continue
		}
		if ev.PatternID != pat.ID {
			t.Errorf("pattern %q: example %q was matched by %q first (ordering problem)",
				pat.ID, pat.Example, ev.PatternID)
		}
	}
}

func TestQueuePositionIsExtracted(t *testing.T) {
	p := loadTestParser(t)
	cases := []struct {
		line string
		want int
	}{
		{"14:22:01 You are in queue position 212", 212},
		{"14:22:01 You are in queue position 1", 1},
		{"14:22:01 Queued 87 / 340", 87},
	}
	for _, c := range cases {
		ev, ok := p.ParseLine(c.line)
		if !ok || ev.Kind != EventQueued {
			t.Fatalf("%q: expected a queued event, got %+v (matched=%v)", c.line, ev, ok)
		}
		if ev.Position != c.want {
			t.Errorf("%q: position = %d, want %d", c.line, ev.Position, c.want)
		}
	}
}

func TestOrdinaryNoiseIsIgnored(t *testing.T) {
	p := loadTestParser(t)
	noise := []string{
		"",
		"Mono config path = 'C:/Program Files/Steam/steamapps/common/Rust/MonoBleedingEdge/etc'",
		"Loading player data",
		"Setting up 8 worker threads for Enlighten.",
	}
	for _, line := range noise {
		if ev, ok := p.ParseLine(line); ok {
			t.Errorf("noise line %q was misread as %s (pattern %q)", line, ev.Kind, ev.PatternID)
		}
	}
}

func TestDetailIsPlainLanguage(t *testing.T) {
	p := loadTestParser(t)
	ev, ok := p.ParseLine("Disconnected: You are banned from this server")
	if !ok {
		t.Fatal("expected a match")
	}
	if ev.Kind != EventRejected {
		t.Fatalf("kind = %s, want rejected", ev.Kind)
	}
	if ev.Detail == "" {
		t.Fatal("detail is empty; the phone would show nothing useful")
	}
}

func TestUnmatchedCaptureLeavesNoPlaceholder(t *testing.T) {
	p := loadTestParser(t)
	ev, ok := p.ParseLine("Disconnected: connection rejected")
	if !ok {
		t.Fatal("expected a match")
	}
	if want := "{reason}"; strings.Contains(ev.Detail, want) {
		t.Errorf("detail %q still contains the raw placeholder %q", ev.Detail, want)
	}
}

// This is the test that stops the simulator and the parser drifting apart.
// Every log line the fake Rust client writes declares what it is supposed to
// mean; the real parser has to agree. When the real Player.log arrives and the
// patterns get rewritten, the scenarios have to be rewritten with them, and
// this test is what proves it was done.
func TestScenarioLinesParseAsDeclared(t *testing.T) {
	p := loadTestParser(t)
	files, err := filepath.Glob("../../testdata/scenarios/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenario files found: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var sc struct {
			Name  string `json:"name"`
			Steps []struct {
				Line           string `json:"line"`
				Expect         string `json:"expect"`
				ExpectPosition int    `json:"expect_position"`
			} `json:"steps"`
		}
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, st := range sc.Steps {
			if st.Line == "" || st.Expect == "" {
				continue
			}
			line := strings.ReplaceAll(st.Line, "{{connect}}", "51.83.128.10:28015")
			ev, ok := p.ParseLine(line)
			if !ok {
				t.Errorf("%s: line %q should mean %q but the parser ignored it", sc.Name, line, st.Expect)
				continue
			}
			if string(ev.Kind) != st.Expect {
				t.Errorf("%s: line %q parsed as %q, scenario says %q", sc.Name, line, ev.Kind, st.Expect)
			}
			if st.ExpectPosition != 0 && ev.Position != st.ExpectPosition {
				t.Errorf("%s: line %q gave position %d, scenario says %d", sc.Name, line, ev.Position, st.ExpectPosition)
			}
		}
	}
}
