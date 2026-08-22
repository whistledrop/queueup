// Package protocol defines every message that travels between the agent on the
// player's PC and the relay in the cloud.
//
// Both sides import this one file, so a message can never mean two different
// things at each end. Everything is JSON: it is easy to read in a log when
// something goes wrong at 3am on wipe day, and easy to extend without breaking
// an older agent.
package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// Version is the wire format version. The agent sends it in Hello; the relay
// refuses anything it does not understand rather than guessing.
const Version = 1

// Type identifies a message.
type Type string

const (
	// Agent to relay.
	TypeHello     Type = "hello"      // first message after connecting
	TypeHeartbeat Type = "heartbeat"  // "still here"
	TypeJobStatus Type = "job_status" // a state change on a running job
	TypeJobLog    Type = "job_log"    // a raw log line, for the debug view

	// Relay to agent.
	TypeWelcome      Type = "welcome"       // hello accepted
	TypeAssign       Type = "assign"        // start (or resume) this job
	TypeCancel       Type = "cancel"        // stop this job and close the game
	TypeServerStatus Type = "server_status" // is the target server up, and how full
	TypeError        Type = "error"         // something was wrong with what you sent
)

// Envelope wraps every message. Payload is decoded according to Type.
type Envelope struct {
	Type    Type            `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Encode wraps a payload in an envelope ready to send.
func Encode(t Type, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encoding %s payload: %w", t, err)
		}
		raw = b
	}
	return json.Marshal(Envelope{Type: t, Payload: raw})
}

// Decode unwraps an envelope's payload into v.
func Decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// Hello is the agent introducing itself. The device token travels in the
// connection's Authorization header, not in here.
type Hello struct {
	ProtocolVersion int    `json:"protocol_version"`
	AgentVersion    string `json:"agent_version"`
	OS              string `json:"os"`
	Hostname        string `json:"hostname"`
	// Simulator is true when the agent is running against the fake Rust client.
	// The relay records it so a test job is never mistaken for a real one.
	Simulator bool `json:"simulator"`
}

// Welcome is the relay accepting the agent.
type Welcome struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	ServerTime time.Time `json:"server_time"`
	// HeartbeatSeconds is how often the relay wants to hear from the agent.
	HeartbeatSeconds int `json:"heartbeat_seconds"`
}

// Job is one join request, as handed to the agent.
type Job struct {
	ID              string `json:"id"`
	ServerAddr      string `json:"server_addr"` // IP:PORT, resolved by the relay at fire time
	ServerName      string `json:"server_name,omitempty"`
	WaitForServerUp bool   `json:"wait_for_server_up"`

	// Resumed is true when the agent is picking this job back up after losing
	// its connection or after the PC rebooted.
	Resumed bool `json:"resumed"`

	// GroupID is unused in v1. It exists so that clan joins, where several
	// people queue for the same server together, do not need a schema change.
	GroupID string `json:"group_id,omitempty"`
}

// Assign tells the agent to run a job.
type Assign struct {
	Job Job `json:"job"`
}

// Cancel tells the agent to stop.
type Cancel struct {
	JobID  string `json:"job_id"`
	Reason string `json:"reason"`
}

// JobStatus is one state change, reported upwards. These are what the phone
// shows, so Detail is always plain language.
type JobStatus struct {
	JobID         string    `json:"job_id"`
	State         string    `json:"state"`
	Position      int       `json:"position,omitempty"`
	Attempt       int       `json:"attempt,omitempty"`
	Detail        string    `json:"detail"`
	ReasonCode    string    `json:"reason_code,omitempty"`
	ReasonMessage string    `json:"reason_message,omitempty"`
	At            time.Time `json:"at"`
}

// JobLog is a raw line from the Rust log, for the debug view only. Never shown
// to a player.
type JobLog struct {
	JobID string    `json:"job_id"`
	Line  string    `json:"line"`
	At    time.Time `json:"at"`
}

// ServerStatus is what the relay sees when it polls the target server. In phase
// 2 this is a stub that always reports online; phase 4 replaces it with real
// A2S and BattleMetrics polling, which is what drives wipe-restart detection.
type ServerStatus struct {
	JobID      string `json:"job_id"`
	Online     bool   `json:"online"`
	Players    int    `json:"players,omitempty"`
	MaxPlayers int    `json:"max_players,omitempty"`
	Queue      int    `json:"queue,omitempty"`
}

// Error is the relay complaining about something the agent sent.
type Error struct {
	Message string `json:"message"`
}

// Heartbeat carries nothing. Its arrival is the whole message.
type Heartbeat struct{}
