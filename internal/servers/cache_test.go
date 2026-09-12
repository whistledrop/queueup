package servers

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// flaky is a provider that can be switched off, the way Steam's server list
// went unreachable on 12 September 2026 while the rest of Steam was fine.
type flaky struct {
	down  bool
	calls int
}

func (f *flaky) Name() string { return "flaky" }

func (f *flaky) ByID(_ context.Context, id string) (Server, error) {
	f.calls++
	if f.down {
		return Server{}, errors.New("context deadline exceeded")
	}
	return Server{ID: id, Name: "Test Server", Address: "10.0.0.1:28015", QueryAddress: id, Online: true}, nil
}

func (f *flaky) Search(_ context.Context, q string, limit int) ([]Server, error) {
	f.calls++
	if f.down {
		return nil, errors.New("context deadline exceeded")
	}
	return []Server{{ID: "10.0.0.1:28010", Name: "Test Server " + q, Address: "10.0.0.1:28015"}}, nil
}

// The point of the whole thing: once we have heard an answer, an outage must
// not stop somebody joining.
func TestASourceOutageFallsBackToWhatWeAlreadyKnew(t *testing.T) {
	f := &flaky{}
	c := NewCached(f)
	ctx := context.Background()

	sv, err := c.ByID(ctx, "10.0.0.1:28010")
	if err != nil || sv.Address != "10.0.0.1:28015" {
		t.Fatalf("first lookup = %+v, %v", sv, err)
	}

	f.down = true
	sv, err = c.ByID(ctx, "10.0.0.1:28010")
	if err != nil {
		t.Fatalf("lookup during an outage failed: %v; a join would have been refused", err)
	}
	if sv.Address != "10.0.0.1:28015" {
		t.Fatalf("stale answer = %+v, want the address we already knew", sv)
	}
}

// Never heard of it, and the source is down: there is nothing honest to return.
func TestAnUnknownServerDuringAnOutageStillFails(t *testing.T) {
	c := NewCached(&flaky{down: true})
	if _, err := c.ByID(context.Background(), "never-seen"); err == nil {
		t.Fatal("invented an answer for a server it has never heard of")
	}
}

func TestSearchFallsBackToTheLastGoodResults(t *testing.T) {
	f := &flaky{}
	c := NewCached(f)
	ctx := context.Background()

	if _, err := c.Search(ctx, "rust", 10); err != nil {
		t.Fatal(err)
	}
	f.down = true
	got, err := c.Search(ctx, "rust", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("search during an outage = %v, %v; want the remembered results", got, err)
	}
}

// A search teaches us addresses. Somebody who searched ten minutes ago must be
// able to join what they found, even though the source has since died.
func TestSearchResultsAreRememberedIndividually(t *testing.T) {
	f := &flaky{}
	c := NewCached(f)
	ctx := context.Background()

	if _, err := c.Search(ctx, "rust", 10); err != nil {
		t.Fatal(err)
	}
	f.down = true
	sv, err := c.ByID(ctx, "10.0.0.1:28010")
	if err != nil {
		t.Fatalf("a server found by search could not be looked up during an outage: %v", err)
	}
	if sv.Address != "10.0.0.1:28015" {
		t.Fatalf("remembered address = %q", sv.Address)
	}
}

// A fresh answer replaces the old one, so a server that genuinely moved is
// followed once the source is healthy again.
func TestAFreshAnswerReplacesTheRememberedOne(t *testing.T) {
	f := &flaky{}
	c := NewCached(f)
	ctx := context.Background()
	if _, err := c.ByID(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	moved := &flaky{}
	c.inner = moved
	sv, err := c.ByID(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Address == "" {
		t.Fatal("a healthy source was ignored in favour of the cache")
	}
	if moved.calls == 0 {
		t.Fatal("the cache answered without asking the source, so it would never notice a change")
	}
}

// The relay runs for weeks. The cache must not grow without bound.
func TestTheCacheIsBounded(t *testing.T) {
	c := NewCached(&flaky{})
	ctx := context.Background()
	for i := 0; i < maxRememberedIDs+50; i++ {
		if _, err := c.ByID(ctx, fmt.Sprintf("server-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	n := len(c.byID)
	c.mu.Unlock()
	if n > maxRememberedIDs {
		t.Fatalf("cache holds %d entries, cap is %d", n, maxRememberedIDs)
	}
}
