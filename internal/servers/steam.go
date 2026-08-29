package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"queueup/internal/a2s"
)

// RustAppID is Rust's Steam application id, used to filter the server list.
const RustAppID = "252490"

// Steam reads Steam's own master server list.
//
// The key is free: https://steamcommunity.com/dev/apikey. Steam does report a
// queue length after all: its "gametype" field carries the same keyword tags a
// direct query returns, Rust's qp tag among them.
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
		Servers []steamServer `json:"servers"`
	} `json:"response"`
}

type steamServer struct {
	// Addr is the QUERY address (ip:queryport), despite the plain name.
	Addr string `json:"addr"`
	// GamePort is what a player actually connects to. For Rust these differ,
	// typically 28015 for the game and 28010-ish for queries.
	GamePort   int    `json:"gameport"`
	Name       string `json:"name"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Map        string `json:"map"`
	// GameType carries the same keyword tags a direct query returns, including
	// Rust's queue length. Free queue counts, no extra request.
	GameType string `json:"gametype"`
}

// toServer turns Steam's record into ours, keeping both addresses straight.
func (s steamServer) toServer() Server {
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		host = s.Addr
	}
	game := s.Addr // fall back to the query address if no game port was given
	if s.GamePort > 0 && host != "" {
		game = net.JoinHostPort(host, strconv.Itoa(s.GamePort))
	}
	return Server{
		// The query address is the id: it is what search returns, and it is
		// stable for as long as the server exists.
		ID:           s.Addr,
		Name:         s.Name,
		Address:      game,
		QueryAddress: s.Addr,
		Online:       true,
		Players:      s.Players,
		MaxPlayers:   s.MaxPlayers,
		Queue:        a2s.QueueFromKeywords(s.GameType),
		Map:          s.Map,
	}
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

// ByID looks a server up by its id, which is its query address.
//
// The gameaddr filter matches on the GAME port, and our id carries the query
// port, so filtering by the full id finds nothing. Filter by IP alone, which
// the filter allows, then pick the one we meant. Machines often host several
// Rust servers on one IP, so the match matters.
func (s *Steam) ByID(ctx context.Context, id string) (Server, error) {
	host, _, err := net.SplitHostPort(id)
	if err != nil {
		host = id
	}
	found, qerr := s.query(ctx, `\appid\`+RustAppID+`\gameaddr\`+host, 50)
	if qerr != nil {
		return Server{}, qerr
	}
	for _, sv := range found {
		if sv.ID == id || sv.Address == id {
			return sv, nil
		}
	}
	if len(found) == 1 {
		// One server on that IP: it is unambiguous even if the port moved.
		return found[0], nil
	}
	return Server{}, fmt.Errorf("that server isn't in Steam's list right now, so it may be down")
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
		// Never the raw error: Go puts the whole request URL in it, and the URL
		// carries the Steam API key. That would write the key into the relay's
		// logs, where it outlives the request and is read by anyone who can see
		// the logs.
		return nil, fmt.Errorf("couldn't reach Steam's server list: %s", scrubKey(err.Error()))
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
		list = append(list, sv.toServer())
	}
	return list, nil
}

// keyInURL matches the API key wherever it appears in a URL or an error string.
var keyInURL = regexp.MustCompile(`([?&]key=)[^&"\s]+`)

// scrubKey removes the Steam API key from text on its way to a log.
func scrubKey(s string) string {
	return keyInURL.ReplaceAllString(s, "${1}REDACTED")
}
