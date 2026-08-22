package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RustAppID is Rust's Steam application id, used to filter the server list.
const RustAppID = "252490"

// Steam reads Steam's own master server list.
//
// The key is free: https://steamcommunity.com/dev/apikey. Steam does not report
// a queue length, so Queue is always zero here; the live queue position comes
// from the game's own log once we are connecting, and phase 4 adds direct A2S
// polling for the rest.
type Steam struct {
	key    string
	client *http.Client
}

// NewSteam builds a Steam-backed provider.
func NewSteam(key string) *Steam {
	return &Steam{key: key, client: &http.Client{Timeout: 15 * time.Second}}
}

// Name identifies this source.
func (s *Steam) Name() string { return "steam" }

type steamResponse struct {
	Response struct {
		Servers []struct {
			Addr       string `json:"addr"`
			Name       string `json:"name"`
			Players    int    `json:"players"`
			MaxPlayers int    `json:"max_players"`
			Map        string `json:"map"`
			Region     int    `json:"region"`
		} `json:"servers"`
	} `json:"response"`
}

// Search asks Steam for Rust servers whose name contains the query.
func (s *Steam) Search(ctx context.Context, query string, limit int) ([]Server, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := `\appid\` + RustAppID + `\empty\1`
	if q := strings.TrimSpace(query); q != "" {
		filter += `\name_match\*` + q + `*`
	}
	return s.query(ctx, filter, limit)
}

// ByID looks a server up by its address. For this source the id IS the address,
// which is why Search returns the address as the id.
func (s *Steam) ByID(ctx context.Context, id string) (Server, error) {
	found, err := s.query(ctx, `\appid\`+RustAppID+`\gameaddr\`+id, 1)
	if err != nil {
		return Server{}, err
	}
	if len(found) == 0 {
		return Server{}, fmt.Errorf("that server isn't in Steam's list right now, so it may be down")
	}
	return found[0], nil
}

func (s *Steam) query(ctx context.Context, filter string, limit int) ([]Server, error) {
	u := "https://api.steampowered.com/IGameServersService/GetServerList/v1/?" +
		url.Values{
			"key":    {s.key},
			"filter": {filter},
			"limit":  {fmt.Sprint(limit)},
		}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach Steam's server list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Steam rejected the API key. Check QUEUEUP_STEAM_API_KEY")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Steam's server list returned %s", resp.Status)
	}

	var out steamResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("couldn't read Steam's reply: %w", err)
	}
	list := make([]Server, 0, len(out.Response.Servers))
	for _, sv := range out.Response.Servers {
		list = append(list, Server{
			ID: sv.Addr, Name: sv.Name, Address: sv.Addr, Online: true,
			Players: sv.Players, MaxPlayers: sv.MaxPlayers, Map: sv.Map,
		})
	}
	return list, nil
}
