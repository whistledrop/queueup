package game

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"queueup/internal/fakerust"
	"queueup/internal/scenario"
)

// SimLauncher runs the fake Rust client instead of the real one. It implements
// exactly the same interface, so the agent cannot tell the difference. Selected
// with the agent's --sim flag.
type SimLauncher struct {
	Scenario *scenario.Scenario
	Log      string
	Speed    float64

	// FailPreflight simulates "Steam isn't running".
	FailPreflight error

	mu      sync.Mutex
	cancel  context.CancelFunc
	exited  chan Exit
	running atomic.Bool
	closing atomic.Bool
}

func (s *SimLauncher) Preflight() error { return s.FailPreflight }
func (s *SimLauncher) LogPath() string  { return s.Log }
func (s *SimLauncher) Running() bool    { return s.running.Load() }

func (s *SimLauncher) Exited() <-chan Exit {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited == nil {
		s.exited = make(chan Exit, 1)
	}
	return s.exited
}

func (s *SimLauncher) Launch(a Addr) error {
	s.mu.Lock()
	if s.running.Load() {
		s.mu.Unlock()
		return errors.New("the fake game is already running")
	}
	if s.exited == nil {
		s.exited = make(chan Exit, 1)
	}
	ch := s.exited
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.closing.Store(false)
	s.running.Store(true)
	s.mu.Unlock()

	go func() {
		err := fakerust.Run(ctx, fakerust.Config{
			Scenario: s.Scenario,
			LogPath:  s.Log,
			Connect:  a.String(),
			Speed:    s.Speed,
		})
		s.running.Store(false)
		ex := Exit{Expected: s.closing.Load()}
		var ee fakerust.ExitError
		if errors.As(err, &ee) {
			ex.Code = ee.Code
		}
		select {
		case ch <- ex:
		default:
		}
	}()
	return nil
}

func (s *SimLauncher) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closing.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}
