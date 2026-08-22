// Package game is the only part of QueueUp that touches Rust, and it is
// deliberately tiny.
//
// ANTI-CHEAT RULE (do not weaken this without asking):
// Rust runs Easy Anti-Cheat. QueueUp is allowed to do exactly four things and
// nothing else:
//  1. launch the game through the Steam URI,
//  2. read the game's log file,
//  3. check whether the game's process is running,
//  4. ask the process to close (the same as the user clicking the X).
//
// No memory reading or writing, no injection, no simulated keyboard or mouse
// input into the game, no touching game files. Anything that needs more than the
// four above is not a feature we build.
package game

import (
	"fmt"
	"net"
	"strconv"
)

// Addr is a Rust server's connection address.
type Addr struct {
	IP   string
	Port int
}

func (a Addr) String() string { return net.JoinHostPort(a.IP, strconv.Itoa(a.Port)) }

// ParseAddr reads "1.2.3.4:28015".
func ParseAddr(s string) (Addr, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return Addr{}, fmt.Errorf("%q is not a valid server address, expected IP:PORT", s)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return Addr{}, fmt.Errorf("%q has an invalid port", s)
	}
	return Addr{IP: host, Port: port}, nil
}

// Exit reports how a game session ended.
type Exit struct {
	Code     int
	Expected bool // true when we asked it to close
}

// Launcher is everything the agent is allowed to do to the game.
type Launcher interface {
	// Preflight checks the obvious blockers (is Steam running?) and returns a
	// plain-language error the user can act on.
	Preflight() error
	// Launch starts the game pointed at a server. It returns once the launch has
	// been handed off, not once the game is connected.
	Launch(a Addr) error
	// Running reports whether the game process is alive.
	Running() bool
	// Close asks the game to exit.
	Close() error
	// Exited fires once per launch, when the game process ends.
	Exited() <-chan Exit
	// LogPath is the file to tail for connection progress.
	LogPath() string
}
