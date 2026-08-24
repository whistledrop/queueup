// Package servers finds Rust servers by name and looks up where they are right
// now.
//
// There are three sources behind one interface, chosen with an environment
// variable, because this turned out to be a decision with money attached:
//
//   - stub          a small built-in list. No account, no key, works offline.
//     This is the default so the app runs out of the box.
//   - steam         Steam's own server list. Needs a free Steam Web API key.
//   - battlemetrics BattleMetrics. Needs a PAID subscription: as of August 2026
//     every one of their API endpoints returns 403 without one,
//     including a single server lookup.
//
// Whichever is chosen, the important part is the same: a server's identity is
// its id, never its address. Rust server addresses change, so the address is
// looked up again at the moment we connect.
package servers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Server is one Rust server, as much as we know about it.
//
// A Rust server has TWO addresses and they are not interchangeable. The game
// port is what a player connects to; the query port is the only one that
// answers status questions. Steam reports the query address as "addr" and the
// game port separately, which is a very easy thing to get wrong: connecting to
// the query port fails, and querying the game port times out.
type Server struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Address is the GAME address, IP:PORT, the one Rust connects to.
	Address string `json:"address"`
	// QueryAddress is where status questions go. Often the game port minus a
	// few. Empty means "same as Address".
	QueryAddress string    `json:"query_address,omitempty"`
	Online       bool      `json:"online"`
	Players      int       `json:"players"`
	MaxPlayers   int       `json:"max_players"`
	Queue        int       `json:"queue"`
	Map          string    `json:"map,omitempty"`
	Region       string    `json:"region,omitempty"`
	LastWipe     time.Time `json:"last_wipe,omitempty"`
}

// PollAddress is where to send status queries for this server.
func (s Server) PollAddress() string {
	if s.QueryAddress != "" {
		return s.QueryAddress
	}
	return s.Address
}

// Provider is a source of server information.
type Provider interface {
	// Name is what the admin view and the logs call this source.
	Name() string
	// Search finds servers by name.
	Search(ctx context.Context, query string, limit int) ([]Server, error)
	// ByID looks one up, and is what resolves an address at connect time.
	ByID(ctx context.Context, id string) (Server, error)
}

// ErrNotConfigured is returned when a source needs a key that has not been set.
type ErrNotConfigured struct {
	Source string
	EnvVar string
	How    string
}

func (e ErrNotConfigured) Error() string {
	return fmt.Sprintf("server search is set to %q but %s is not set. %s", e.Source, e.EnvVar, e.How)
}

// FromEnv builds the provider named by QUEUEUP_SERVER_SOURCE.
func FromEnv() (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("QUEUEUP_SERVER_SOURCE"))) {
	case "", "stub":
		return NewStub(), nil
	case "steam":
		key := os.Getenv("QUEUEUP_STEAM_API_KEY")
		if key == "" {
			return nil, ErrNotConfigured{
				Source: "steam", EnvVar: "QUEUEUP_STEAM_API_KEY",
				How: "Get a free key at https://steamcommunity.com/dev/apikey",
			}
		}
		return NewSteam(key), nil
	case "battlemetrics":
		token := os.Getenv("QUEUEUP_BATTLEMETRICS_TOKEN")
		if token == "" {
			return nil, ErrNotConfigured{
				Source: "battlemetrics", EnvVar: "QUEUEUP_BATTLEMETRICS_TOKEN",
				How: "BattleMetrics API access needs a paid subscription. Create a token at https://www.battlemetrics.com/developers",
			}
		}
		return NewBattleMetrics(token), nil
	default:
		return nil, fmt.Errorf("QUEUEUP_SERVER_SOURCE should be stub, steam or battlemetrics")
	}
}
