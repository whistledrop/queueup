// Package serverstat reports what the target Rust server is doing.
//
// In production this runs on the relay, not on the player's PC, and will use
// direct A2S queries for fast polling plus BattleMetrics for search and
// metadata (phase 4). For phase 1 there is a scripted fake and an always-up
// stub, which is all the agent's state machine needs to be exercised.
package serverstat

import (
	"sync"
	"time"

	"queueup/internal/scenario"
)

// Status is one observation of the server.
type Status struct {
	Online     bool
	Players    int
	MaxPlayers int
	Queue      int
	At         time.Time
}

// Source is anything that can tell us the server's current state.
type Source interface {
	Poll() Status
}

// AlwaysUp is the stub used when a job is not waiting for a wipe restart.
type AlwaysUp struct{}

func (AlwaysUp) Poll() Status { return Status{Online: true, At: time.Now()} }

// Scripted replays a scenario's server timeline: down, then up, then maybe down
// again. This is how the wipe-day flapping cases get tested.
type Scripted struct {
	points []scenario.ServerPoint
	now    func() time.Time

	mu    sync.Mutex
	start time.Time
	speed float64
}

// NewScripted builds a scripted source. speed>1 compresses the timeline.
func NewScripted(points []scenario.ServerPoint, speed float64) *Scripted {
	if speed <= 0 {
		speed = 1
	}
	return &Scripted{points: points, now: time.Now, start: time.Now(), speed: speed}
}

// Poll returns the state for the current point on the timeline. Before the
// first point, the server is considered down.
func (s *Scripted) Poll() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	elapsed := time.Duration(float64(now.Sub(s.start)) * s.speed)

	cur := Status{Online: false, At: now}
	var acc time.Duration
	for _, p := range s.points {
		acc += time.Duration(p.AfterMs) * time.Millisecond
		if elapsed < acc {
			break
		}
		cur = Status{Online: p.Online, Players: p.Players, MaxPlayers: p.Max, Queue: p.Queue, At: now}
	}
	return cur
}
