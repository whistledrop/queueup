package relay

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"queueup/internal/a2s"
	"queueup/internal/servers"
)

// The free server checker on the website: search a Rust server and see
// whether it is up, how full it is, how long its queue is, and how long it
// has been running since its last restart or wipe.
//
// Open to anybody, without an account, so three rules keep it from being
// anything else:
//
//   - It only ever talks to servers in the public server list. A status check
//     takes a server id, looks it up in the list, and asks the address the
//     list gave. An address typed by a stranger is never contacted, so this
//     cannot be used to send traffic at somebody's machine.
//   - Each address on the internet gets a modest number of lookups a minute.
//   - Answers are kept for half a minute, so a popular server checked by a
//     hundred people is asked once.

const (
	publicSearchLimit = 30
	publicStatusLimit = 60
	publicWindow      = time.Minute
	publicCacheFor    = 30 * time.Second
)

type cachedStatus struct {
	at   time.Time
	body map[string]any
}

type publicCache struct {
	mu sync.Mutex
	m  map[string]cachedStatus
}

func (c *publicCache) get(key string) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	if !ok || time.Since(v.at) > publicCacheFor {
		return nil, false
	}
	return v.body, true
}

func (c *publicCache) put(key string, body map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 5000 {
		c.m = map[string]cachedStatus{} // bounded: start again rather than grow
	}
	c.m[key] = cachedStatus{at: time.Now(), body: body}
}

func (s *Server) publicToolRoutes() {
	s.publicSearches = newThrottle(publicSearchLimit, publicWindow, time.Now)
	s.publicStatuses = newThrottle(publicStatusLimit, publicWindow, time.Now)
	s.mux.HandleFunc("GET /api/public/servers", s.handlePublicSearch)
	s.mux.HandleFunc("GET /api/public/servers/{id}", s.handlePublicStatus)
}

// publicServer is what the website is told about one server: no more than the
// public server list already says to anybody who asks Steam.
func publicServer(sv servers.Server) map[string]any {
	return map[string]any{
		"id": sv.ID, "name": sv.Name, "address": sv.Address, "online": sv.Online,
		"players": sv.Players, "max_players": sv.MaxPlayers, "queue": sv.Queue,
		"map": sv.Map, "region": sv.Region,
	}
}

func (s *Server) handlePublicSearch(w http.ResponseWriter, r *http.Request) {
	from := "ip:" + s.clientIP(r)
	if s.publicSearches.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "That's a lot of searches. Give it a minute.")
		return
	}
	s.publicSearches.fail(from)
	if s.cfg.Servers == nil {
		writeError(w, http.StatusServiceUnavailable, "Server search isn't available right now.")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeError(w, http.StatusBadRequest, "Type at least two letters of the server's name.")
		return
	}
	if len(q) > 80 {
		q = q[:80]
	}
	key := "search:" + strings.ToLower(q)
	if body, ok := s.publicLookups.get(key); ok {
		writeJSON(w, http.StatusOK, body)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	found, err := s.cfg.Servers.Search(ctx, q, 40)
	if err != nil {
		s.log.Error("public server search", "err", err)
		writeError(w, http.StatusBadGateway, "The public server list isn't answering. Try again in a moment.")
		return
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Online != found[j].Online {
			return found[i].Online
		}
		return found[i].Players+found[i].Queue > found[j].Players+found[j].Queue
	})
	if len(found) > 25 {
		found = found[:25]
	}
	out := make([]map[string]any, 0, len(found))
	for _, sv := range found {
		out = append(out, publicServer(sv))
	}
	body := map[string]any{"servers": out}
	s.publicLookups.put(key, body)
	writeJSON(w, http.StatusOK, body)
}

// handlePublicStatus is one server, asked directly: the list's numbers can be
// minutes old, and only the server itself says when it last started.
func (s *Server) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	from := "ip:" + s.clientIP(r)
	if s.publicStatuses.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "That's a lot of checks. Give it a minute.")
		return
	}
	s.publicStatuses.fail(from)
	if s.cfg.Servers == nil {
		writeError(w, http.StatusServiceUnavailable, "Server checks aren't available right now.")
		return
	}
	id := r.PathValue("id")
	key := "status:" + id
	if body, ok := s.publicLookups.get(key); ok {
		writeJSON(w, http.StatusOK, body)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	sv, err := s.cfg.Servers.ByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "That server isn't in the public list right now. It may be down, or restarting for a wipe.")
		return
	}
	body := publicServer(sv)
	body["checked_at"] = time.Now().UTC()
	// The list's address, never one from the request.
	info, err := a2s.Query(ctx, sv.PollAddress(), 3*time.Second)
	if err != nil {
		body["online"] = false
		body["answered"] = false
	} else {
		body["online"], body["answered"] = true, true
		body["players"], body["max_players"] = info.Players, info.MaxPlayers
		body["queue"] = info.Queue
		if info.Map != "" {
			body["map"] = info.Map
		}
		if !info.BornAt.IsZero() {
			body["up_since"] = info.BornAt
		}
	}
	s.publicLookups.put(key, body)
	writeJSON(w, http.StatusOK, body)
}
