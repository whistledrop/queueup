// Package relay is the cloud half of QueueUp: it holds the agents' connections
// open, stores jobs, and lets the web app talk to a PC it can never reach
// directly.
package relay

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"queueup/internal/protocol"
	"queueup/internal/store"
)

// ErrAgentOffline means the PC is not currently connected. It is a completely
// normal condition, not a failure: the job waits in the database until the agent
// comes back.
var ErrAgentOffline = errors.New("that PC isn't connected right now")

// AgentConn is one live connection from one PC.
//
// The agent dialled out to us. Nothing ever connects in to the player's machine,
// which is why QueueUp needs no port forwarding and no router configuration.
type AgentConn struct {
	DeviceID  string
	AccountID string
	Since     time.Time
	Simulator bool

	conn   *websocket.Conn
	send   chan []byte
	ctx    context.Context
	cancel context.CancelFunc
	log    *slog.Logger
}

// Send queues a message for this agent. It never blocks: if the agent is so far
// behind that its queue is full, the connection is dropped and the agent
// reconnects, which is cleaner than stalling the relay.
func (a *AgentConn) Send(t protocol.Type, payload any) error {
	raw, err := protocol.Encode(t, payload)
	if err != nil {
		return err
	}
	select {
	case a.send <- raw:
		return nil
	case <-a.ctx.Done():
		return ErrAgentOffline
	default:
		a.log.Warn("agent send queue full, dropping connection", "device", a.DeviceID)
		a.cancel()
		return ErrAgentOffline
	}
}

// writePump is the only goroutine that writes to the socket.
func (a *AgentConn) writePump() {
	defer a.cancel()
	for {
		select {
		case <-a.ctx.Done():
			return
		case raw := <-a.send:
			ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
			err := a.conn.Write(ctx, websocket.MessageText, raw)
			cancel()
			if err != nil {
				a.log.Debug("write failed, closing", "device", a.DeviceID, "err", err)
				return
			}
		}
	}
}

// Hub tracks which PCs are connected right now, and fans job updates out to
// whoever is watching from a browser.
type Hub struct {
	log *slog.Logger

	mu     sync.RWMutex
	agents map[string]*AgentConn // by device id

	subMu   sync.Mutex
	subs    map[string]map[int64]chan store.Event // by job id
	nextSub int64
}

// NewHub builds an empty hub.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:    log,
		agents: map[string]*AgentConn{},
		subs:   map[string]map[int64]chan store.Event{},
	}
}

// Register adds a connection, replacing any existing one for the same PC.
// One account, one PC, one connection: a second connection means the first is
// stale, so the old one goes.
func (h *Hub) Register(a *AgentConn) {
	h.mu.Lock()
	old := h.agents[a.DeviceID]
	h.agents[a.DeviceID] = a
	h.mu.Unlock()

	if old != nil {
		h.log.Info("replacing an older connection for this PC", "device", a.DeviceID)
		old.cancel()
	}
	go a.writePump()
}

// Unregister removes a connection, but only if it is still the current one.
func (h *Hub) Unregister(a *AgentConn) {
	h.mu.Lock()
	if h.agents[a.DeviceID] == a {
		delete(h.agents, a.DeviceID)
	}
	h.mu.Unlock()
	a.cancel()
}

// Agent returns the live connection for a PC, if it has one.
func (h *Hub) Agent(deviceID string) (*AgentConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	a, ok := h.agents[deviceID]
	return a, ok
}

// Online reports whether a PC is connected.
func (h *Hub) Online(deviceID string) bool {
	_, ok := h.Agent(deviceID)
	return ok
}

// SendTo delivers a message to a PC, or reports that it is offline.
func (h *Hub) SendTo(deviceID string, t protocol.Type, payload any) error {
	a, ok := h.Agent(deviceID)
	if !ok {
		return ErrAgentOffline
	}
	return a.Send(t, payload)
}

// Connections lists every connected PC, for the admin view.
func (h *Hub) Connections() []*AgentConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*AgentConn, 0, len(h.agents))
	for _, a := range h.agents {
		out = append(out, a)
	}
	return out
}

// Subscribe opens a feed of events for one job. Used by the live status screen.
// The returned function must be called to stop listening.
func (h *Hub) Subscribe(jobID string) (<-chan store.Event, func()) {
	ch := make(chan store.Event, 32)
	h.subMu.Lock()
	h.nextSub++
	id := h.nextSub
	if h.subs[jobID] == nil {
		h.subs[jobID] = map[int64]chan store.Event{}
	}
	h.subs[jobID][id] = ch
	h.subMu.Unlock()

	return ch, func() {
		h.subMu.Lock()
		defer h.subMu.Unlock()
		if m := h.subs[jobID]; m != nil {
			if c, ok := m[id]; ok {
				delete(m, id)
				close(c)
			}
			if len(m) == 0 {
				delete(h.subs, jobID)
			}
		}
	}
}

// Publish pushes an event to everyone watching that job. A watcher that has
// stopped reading is skipped rather than allowed to block the relay.
func (h *Hub) Publish(e store.Event) {
	h.subMu.Lock()
	defer h.subMu.Unlock()
	for _, ch := range h.subs[e.JobID] {
		select {
		case ch <- e:
		default:
		}
	}
}

// DisconnectAll drops every agent connection. Tests use it to simulate the
// relay process dying (which severs all sockets); an admin can use it to force
// a clean reconnect wave after a config change.
func (h *Hub) DisconnectAll() {
	for _, a := range h.Connections() {
		a.cancel()
	}
}
