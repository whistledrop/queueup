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

// BattleMetrics reads the BattleMetrics API.
//
// IMPORTANT: as of August 2026 this API requires a PAID subscription. Without a
// token every endpoint returns 403, including looking up a single server. That
// is why it is not the default. It is still the best source when it is paid for:
// it is the only one of the three that reports a server's queue length, which is
// exactly the number a wipe-day tool wants.
type BattleMetrics struct {
	token  string
	client *http.Client
}

// NewBattleMetrics builds a BattleMetrics-backed provider.
func NewBattleMetrics(token string) *BattleMetrics {
	return &BattleMetrics{token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

// Name identifies this source.
func (b *BattleMetrics) Name() string { return "battlemetrics" }

type bmServer struct {
	ID         string `json:"id"`
	Attributes struct {
		Name       string `json:"name"`
		IP         string `json:"ip"`
		Port       int    `json:"port"`
		Players    int    `json:"players"`
		MaxPlayers int    `json:"maxPlayers"`
		Status     string `json:"status"`
		Country    string `json:"country"`
		Details    struct {
			Map           string `json:"map"`
			QueuedPlayers int    `json:"rust_queued_players"`
			LastWipe      string `json:"rust_last_wipe"`
		} `json:"details"`
	} `json:"attributes"`
}

func (s bmServer) toServer() Server {
	out := Server{
		ID:         s.ID,
		Name:       s.Attributes.Name,
		Online:     s.Attributes.Status == "online",
		Players:    s.Attributes.Players,
		MaxPlayers: s.Attributes.MaxPlayers,
		Queue:      s.Attributes.Details.QueuedPlayers,
		Map:        s.Attributes.Details.Map,
		Region:     s.Attributes.Country,
	}
	if s.Attributes.IP != "" && s.Attributes.Port != 0 {
		out.Address = fmt.Sprintf("%s:%d", s.Attributes.IP, s.Attributes.Port)
	}
	if t, err := time.Parse(time.RFC3339, s.Attributes.Details.LastWipe); err == nil {
		out.LastWipe = t
	}
	return out
}

// Search finds Rust servers by name.
func (b *BattleMetrics) Search(ctx context.Context, query string, limit int) ([]Server, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{
		"filter[game]": {"rust"},
		"page[size]":   {fmt.Sprint(limit)},
	}
	if s := strings.TrimSpace(query); s != "" {
		q.Set("filter[search]", s)
	}

	var body struct {
		Data []bmServer `json:"data"`
	}
	if err := b.get(ctx, "https://api.battlemetrics.com/servers?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(body.Data))
	for _, s := range body.Data {
		out = append(out, s.toServer())
	}
	return out, nil
}

// ByID looks one server up. This is the call that resolves a server's current
// address just before we connect, which matters because Rust server IPs change.
func (b *BattleMetrics) ByID(ctx context.Context, id string) (Server, error) {
	var body struct {
		Data bmServer `json:"data"`
	}
	if err := b.get(ctx, "https://api.battlemetrics.com/servers/"+url.PathEscape(id), &body); err != nil {
		return Server{}, err
	}
	if body.Data.ID == "" {
		return Server{}, fmt.Errorf("we couldn't find that server")
	}
	return body.Data.toServer(), nil
}

func (b *BattleMetrics) get(ctx context.Context, u string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach BattleMetrics: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusUnauthorized:
		return fmt.Errorf("BattleMetrics refused the request. Their API needs a paid subscription; check QUEUEUP_BATTLEMETRICS_TOKEN")
	case http.StatusTooManyRequests:
		return fmt.Errorf("BattleMetrics is rate limiting us. Try again in a moment")
	case http.StatusNotFound:
		return fmt.Errorf("we couldn't find that server")
	default:
		return fmt.Errorf("BattleMetrics returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("couldn't read the BattleMetrics reply: %w", err)
	}
	return nil
}
