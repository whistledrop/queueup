package servers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Stub is a small built-in list of made-up servers. It needs no account, no key
// and no internet, so the web app works the moment you clone the repo, and the
// tests never depend on somebody else's API being up.
//
// The player counts drift slightly over time so the live status screen visibly
// does something.
type Stub struct {
	mu      sync.Mutex
	servers []Server
	started time.Time
}

// NewStub builds the built-in list.
func NewStub() *Stub {
	return &Stub{started: time.Now(), servers: []Server{
		{ID: "stub-1", Name: "Rustopia EU Main", Address: "51.83.128.10:28015",
			QueryAddress: "51.83.128.10:28010",
			Online:       true, Players: 198, MaxPlayers: 200, Queue: 312, Map: "Procedural", Region: "EU"},
		{ID: "stub-2", Name: "Rusty Moose |EU Main", Address: "51.83.128.20:28015",
			Online: true, Players: 175, MaxPlayers: 250, Queue: 0, Map: "Procedural", Region: "EU"},
		{ID: "stub-3", Name: "Rustafied EU Trio", Address: "45.62.160.30:28015",
			Online: true, Players: 120, MaxPlayers: 150, Queue: 8, Map: "Procedural", Region: "EU"},
		{ID: "stub-4", Name: "Rustafied US Main", Address: "45.62.160.40:28015",
			Online: true, Players: 210, MaxPlayers: 250, Queue: 45, Map: "Procedural", Region: "US"},
		{ID: "stub-5", Name: "Pickle Vanilla EU", Address: "51.83.128.50:28015",
			Online: false, Players: 0, MaxPlayers: 200, Queue: 0, Map: "Procedural", Region: "EU"},
		{ID: "stub-6", Name: "Bloo Lagoon EU Duo", Address: "51.83.128.60:28015",
			Online: true, Players: 88, MaxPlayers: 100, Queue: 0, Map: "Procedural", Region: "EU"},
	}}
}

// Name identifies this source.
func (s *Stub) Name() string { return "stub" }

// Search does a plain case-insensitive name match.
func (s *Stub) Search(_ context.Context, query string, limit int) ([]Server, error) {
	if limit <= 0 {
		limit = 20
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := []Server{}
	for _, sv := range s.all() {
		if q == "" || strings.Contains(strings.ToLower(sv.Name), q) {
			out = append(out, sv)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ByID looks one up.
func (s *Stub) ByID(_ context.Context, id string) (Server, error) {
	for _, sv := range s.all() {
		if sv.ID == id {
			return sv, nil
		}
	}
	return Server{}, fmt.Errorf("we couldn't find that server")
}

// all returns the list with the player counts nudged, so the numbers on screen
// are not frozen.
func (s *Stub) all() []Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	drift := int(time.Since(s.started).Seconds()) % 9
	out := make([]Server, len(s.servers))
	copy(out, s.servers)
	for i := range out {
		if !out[i].Online {
			continue
		}
		out[i].Players = clamp(out[i].Players-4+drift, 0, out[i].MaxPlayers)
		if out[i].Queue > 0 {
			out[i].Queue = clamp(out[i].Queue-4+drift, 0, 9999)
		}
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
