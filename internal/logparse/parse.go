// Package logparse turns raw Rust client log lines into typed events.
//
// The log wording itself is NOT in this file. It lives in configs/patterns.json.
// Rust patches change log text roughly monthly; when that happens the fix is to
// edit that JSON file and restart the agent. No Go code changes, no rebuild.
package logparse

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// EventKind is what a log line means, in terms the join state machine cares about.
type EventKind string

const (
	EventConnecting   EventKind = "connecting"
	EventQueued       EventKind = "queued"
	EventJoined       EventKind = "joined"
	EventDisconnected EventKind = "disconnected"
	EventRejected     EventKind = "rejected"
	EventServerFull   EventKind = "server_full"
	EventSteamProblem EventKind = "steam_problem"
	// EventUserQuit is the game announcing a graceful shutdown. A crash does not
	// write these lines, which is exactly what lets the agent tell "the player
	// closed Rust on purpose" apart from "Rust died and needs relaunching".
	EventUserQuit EventKind = "user_quit"
)

var validKinds = map[EventKind]bool{
	EventConnecting: true, EventQueued: true, EventJoined: true,
	EventDisconnected: true, EventRejected: true, EventServerFull: true,
	EventSteamProblem: true, EventUserQuit: true,
}

// Event is one parsed log line.
type Event struct {
	Kind      EventKind
	Position  int    // queue position, 0 if unknown or not applicable
	Detail    string // plain-language text, safe to show a user
	PatternID string // which pattern matched, for debugging
	Raw       string // the original log line
}

// Pattern is one entry from configs/patterns.json.
type Pattern struct {
	ID       string    `json:"id"`
	Event    EventKind `json:"event"`
	Regex    string    `json:"regex"`
	Detail   string    `json:"detail"`
	Example  string    `json:"example"`
	Verified bool      `json:"verified"`
}

type patternFile struct {
	Version  int       `json:"version"`
	Patterns []Pattern `json:"patterns"`
}

// Parser matches log lines against the configured patterns, in file order.
// First match wins.
type Parser struct {
	version  int
	patterns []Pattern
	res      []*regexp.Regexp
}

// Load reads and compiles a pattern file.
func Load(path string) (*Parser, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pattern file: %w", err)
	}
	return Parse(raw)
}

// Parse compiles a pattern file that has already been read into memory.
func Parse(raw []byte) (*Parser, error) {
	var pf patternFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return nil, fmt.Errorf("pattern file is not valid JSON: %w", err)
	}
	if len(pf.Patterns) == 0 {
		return nil, fmt.Errorf("pattern file contains no patterns")
	}
	p := &Parser{version: pf.Version, patterns: pf.Patterns}
	seen := map[string]bool{}
	for _, pat := range pf.Patterns {
		if pat.ID == "" {
			return nil, fmt.Errorf("a pattern is missing its id")
		}
		if seen[pat.ID] {
			return nil, fmt.Errorf("duplicate pattern id %q", pat.ID)
		}
		seen[pat.ID] = true
		if !validKinds[pat.Event] {
			return nil, fmt.Errorf("pattern %q has unknown event %q", pat.ID, pat.Event)
		}
		re, err := regexp.Compile(pat.Regex)
		if err != nil {
			return nil, fmt.Errorf("pattern %q has a bad regex: %w", pat.ID, err)
		}
		p.res = append(p.res, re)
	}
	return p, nil
}

// Version reports the pattern file version, for logging.
func (p *Parser) Version() int { return p.version }

// Unverified lists pattern IDs whose wording has not yet been checked against a
// real Player.log. The agent warns about these on startup.
func (p *Parser) Unverified() []string {
	var out []string
	for _, pat := range p.patterns {
		if !pat.Verified {
			out = append(out, pat.ID)
		}
	}
	return out
}

// ParseLine matches one log line. ok is false for the vast majority of lines,
// which are noise we do not care about.
func (p *Parser) ParseLine(line string) (Event, bool) {
	for i, re := range p.res {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pat := p.patterns[i]
		caps := map[string]string{}
		for gi, name := range re.SubexpNames() {
			if name != "" && gi < len(m) {
				caps[name] = strings.TrimSpace(m[gi])
			}
		}
		ev := Event{
			Kind:      pat.Event,
			Detail:    expand(pat.Detail, caps),
			PatternID: pat.ID,
			Raw:       line,
		}
		if v, err := strconv.Atoi(caps["position"]); err == nil {
			ev.Position = v
		}
		return ev, true
	}
	return Event{}, false
}

// expand replaces {name} in a detail template with captured groups, and tidies
// up whitespace left behind by groups that did not match.
func expand(tmpl string, caps map[string]string) string {
	out := tmpl
	for k, v := range caps {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	// Drop any placeholders with no matching capture.
	out = regexp.MustCompile(`\{[a-zA-Z_][a-zA-Z0-9_]*\}`).ReplaceAllString(out, "")
	return strings.TrimSpace(strings.Join(strings.Fields(out), " "))
}
