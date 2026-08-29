package relayclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"queueup/internal/protocol"
)

type quietHandler struct{}

func (quietHandler) OnConnected(protocol.Welcome)         {}
func (quietHandler) OnDisconnected(error)                 {}
func (quietHandler) OnAssign(protocol.Job)                {}
func (quietHandler) OnCancel(protocol.Cancel)             {}
func (quietHandler) OnServerStatus(protocol.ServerStatus) {}

func testClient(url string) *Client {
	return &Client{
		RelayURL: url, DeviceToken: "a-token", AgentVersion: "test",
		Handler: quietHandler{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxBackoff: 20 * time.Millisecond,
	}
}

// Being told "you are not paired" makes the agent erase its pairing, which is
// the only sensible response but also unrecoverable without somebody standing
// at the PC. A relay can say that for reasons which have nothing to do with the
// pairing: mid-deploy, a database it cannot read, or a hotel wifi answering on
// its behalf. So one rejection is not enough to throw away the pairing.
func TestOneRejectionDoesNotThrowAwayThePairing(t *testing.T) {
	var hits int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		http.Error(w, `{"error":"nope"}`, http.StatusForbidden)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := testClient(ts.URL).Run(ctx)
	if !errors.Is(err, ErrUnlinked) {
		t.Fatalf("err = %v, want ErrUnlinked once it is sure", err)
	}
	if n := atomic.LoadInt64(&hits); n < 3 {
		t.Fatalf("gave up after %d rejection(s). A passing 403 would have unpaired the PC.", n)
	}
}

// A relay that is merely having a bad day must never be mistaken for one that
// has unlinked us: 503 is retried forever, and the pairing is left alone.
func TestATemporaryRefusalIsRetriedAndNeverUnlinks(t *testing.T) {
	var hits int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		http.Error(w, `{"error":"come back later"}`, http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	if err := testClient(ts.URL).Run(ctx); err != nil {
		t.Fatalf("err = %v; a 503 must never end the agent", err)
	}
	if n := atomic.LoadInt64(&hits); n < 2 {
		t.Fatalf("only tried %d time(s) in the window; it should keep trying", n)
	}
}
