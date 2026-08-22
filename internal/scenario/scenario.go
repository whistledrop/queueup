// Package scenario describes a scripted wipe-day situation used for testing
// without the real game: what the Rust client writes to its log and when, and
// what the target server is doing at the same time.
package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Scenario is one testable situation, e.g. "long queue" or "crash mid queue".
type Scenario struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Server      []ServerPoint `json:"server"` // optional; empty means "always up"
	Steps       []Step        `json:"steps"`
}

// Step is one thing the fake Rust client does, AfterMs after the previous step.
type Step struct {
	AfterMs int    `json:"after_ms"`
	Line    string `json:"line"` // written to the fake Player.log; {{connect}} is substituted
	Exit    *int   `json:"exit"` // if set, the fake game exits with this code (crash simulation)

	// Expect and ExpectPosition are not used at runtime. They are the contract
	// checked by the parser test: this line MUST parse to this event. That test
	// is what stops the simulator and the parser drifting apart.
	Expect         string `json:"expect"`
	ExpectPosition int    `json:"expect_position"`
}

func (s Step) Delay() time.Duration { return time.Duration(s.AfterMs) * time.Millisecond }

// ServerPoint is the target server's state from AfterMs onwards.
type ServerPoint struct {
	AfterMs int  `json:"after_ms"`
	Online  bool `json:"online"`
	Players int  `json:"players"`
	Max     int  `json:"max"`
	Queue   int  `json:"queue"`
}

// Load reads one scenario file.
func Load(path string) (*Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario: %w", err)
	}
	var s Scenario
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("scenario %s is not valid JSON: %w", path, err)
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("scenario %s has no steps", path)
	}
	return &s, nil
}
