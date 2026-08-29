package relay

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"queueup/internal/servers"
	"queueup/internal/store"
)

// dialAgent asks to open an agent connection with a token, and returns the
// status code the relay answered with before any upgrade happens.
func dialAgent(t *testing.T, ts *httptest.Server, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// An agent that is told 401 or 403 concludes it has been unlinked and ERASES
// its pairing, because that is the only sane response to "you are not who you
// say you are". So the relay must only say that when it is true. A relay that
// cannot read its own database does not know who is calling, and answering
// "you are not paired" would unpair a customer's PC from a thousand miles away
// over a momentary disk problem.
func TestAnUnreadableDatabaseDoesNotTellAgentsTheyAreUnpaired(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := st.CreateAccount("player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.StartPairing("PC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimPairingCode(acct.ID, p.Code); err != nil {
		t.Fatal(err)
	}
	token, done, err := st.CollectPairingResult(p.ClaimToken)
	if err != nil || !done {
		t.Fatalf("pairing did not complete: %v", err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{Store: st, Log: quiet, Servers: servers.NewStub()})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// The database goes away underneath the relay.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	code := dialAgent(t, ts, token)
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		t.Fatalf("status = %d: the relay told a correctly paired PC it was not paired, "+
			"because it could not read the database. That erases the pairing.", code)
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 so the agent simply tries again later", code)
	}
}

// A token that genuinely is not ours must still be refused outright, or an
// unlinked PC would keep knocking forever.
func TestAnUnknownTokenIsStillRefused(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(New(Config{Store: st, Log: quiet, Servers: servers.NewStub()}))
	defer ts.Close()

	if code := dialAgent(t, ts, "not-a-real-token"); code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a token we have never seen", code)
	}
	if code := dialAgent(t, ts, ""); code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when no token is offered at all", code)
	}
}
