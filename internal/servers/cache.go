package servers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Cached remembers what a provider last told us, and falls back to that when
// the provider stops answering.
//
// Learned the hard way on 12 September 2026: Steam's server list endpoint hung
// while the rest of Steam was perfectly healthy. Search broke, which is
// annoying, but joining broke too, because a job created from a server id has
// to look the address up first. QueueUp was holding a database full of server
// addresses and still could not start a single join.
//
// A stale address is worth far more than no join at all: addresses change
// between wipes, not between minutes, and if a remembered one is wrong the
// connect simply fails and retries. So on any provider error this serves the
// last good answer, however old, and only gives up when it has never heard of
// the thing being asked about.
type Cached struct {
	inner Provider
	now   func() time.Time

	mu       sync.Mutex
	byID     map[string]cacheEntry
	searches map[string]cacheEntry
}

type cacheEntry struct {
	servers []Server
	at      time.Time
}

// Remembering everything forever would be a slow leak on a long-running relay.
// These caps are generous next to any real number of servers people use.
const (
	maxRememberedIDs      = 2000
	maxRememberedSearches = 500
)

// NewCached wraps a provider.
func NewCached(inner Provider) *Cached {
	return &Cached{
		inner:    inner,
		now:      time.Now,
		byID:     map[string]cacheEntry{},
		searches: map[string]cacheEntry{},
	}
}

// Name identifies the wrapped source, so logs and the admin view read the same
// as before.
func (c *Cached) Name() string { return c.inner.Name() }

// ByID looks a server up, falling back to the last good answer.
func (c *Cached) ByID(ctx context.Context, id string) (Server, error) {
	sv, err := c.inner.ByID(ctx, id)
	if err == nil {
		c.remember(c.byID, id, []Server{sv}, maxRememberedIDs)
		return sv, nil
	}
	if cached, ok := c.recall(c.byID, id); ok && len(cached) > 0 {
		return cached[0], nil
	}
	return Server{}, err
}

// Search finds servers, falling back to the last good answer for this query.
func (c *Cached) Search(ctx context.Context, query string, limit int) ([]Server, error) {
	key := fmt.Sprintf("%s\x00%d", query, limit)
	found, err := c.inner.Search(ctx, query, limit)
	if err == nil {
		c.remember(c.searches, key, found, maxRememberedSearches)
		// A search is also the best chance to learn addresses, so every result
		// is remembered individually. That is what lets somebody join a server
		// they found ten minutes ago even though the source has since died.
		for _, sv := range found {
			c.remember(c.byID, sv.ID, []Server{sv}, maxRememberedIDs)
		}
		return found, nil
	}
	if cached, ok := c.recall(c.searches, key); ok {
		return cached, nil
	}
	return nil, err
}

// Stale reports whether the last attempt for this id had to fall back. Used by
// callers that want to say so.
func (c *Cached) Stale(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.byID[id]
	return ok
}

func (c *Cached) remember(into map[string]cacheEntry, key string, servers []Server, cap int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(into) >= cap {
		// Drop the oldest thing we know. Precise enough for a cache, and far
		// cheaper than keeping a sorted structure.
		var oldestKey string
		var oldest time.Time
		for k, e := range into {
			if oldest.IsZero() || e.at.Before(oldest) {
				oldestKey, oldest = k, e.at
			}
		}
		delete(into, oldestKey)
	}
	into[key] = cacheEntry{servers: servers, at: c.now()}
}

func (c *Cached) recall(from map[string]cacheEntry, key string) ([]Server, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := from[key]
	if !ok {
		return nil, false
	}
	return e.servers, true
}
