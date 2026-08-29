package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"queueup/internal/relay"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// These cover what the web app does: sign in, search for a server, star it, and
// start a join from a server id rather than an address.

// post sends an unauthenticated request, the way the sign-in page does.
func (h *harness) post(path string, body any) (int, map[string]any) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := h.http.Client().Post(h.http.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestRegisterThenUseTheSessionForEverythingElse(t *testing.T) {
	h := newHarness(t)

	code, out := h.post("/api/auth/register", map[string]string{
		"email": "new@example.com", "password": "correct horse battery",
	})
	if code != http.StatusCreated {
		t.Fatalf("register returned %d: %v", code, out)
	}
	session, _ := out["session_token"].(string)
	if session == "" {
		t.Fatal("no session token came back")
	}

	// The session works on a normal API call, exactly as the web app uses it.
	h.acctToken = session
	status, devices := h.call(http.MethodGet, "/api/devices", nil)
	if status != http.StatusOK {
		t.Fatalf("listing devices with a session returned %d: %v", status, devices)
	}

	status, me := h.call(http.MethodGet, "/api/auth/me", nil)
	if status != http.StatusOK || me["email"] != "new@example.com" {
		t.Fatalf("who am I returned %d: %v", status, me)
	}
}

func TestSignInAndOut(t *testing.T) {
	h := newHarness(t)
	h.post("/api/auth/register", map[string]string{
		"email": "player@queueup.test", "password": "correct horse battery",
	})

	code, out := h.post("/api/auth/login", map[string]string{
		"email": "player@queueup.test", "password": "correct horse battery",
	})
	if code != http.StatusOK {
		t.Fatalf("sign in returned %d: %v", code, out)
	}
	session := out["session_token"].(string)

	h.acctToken = session
	if status, _ := h.call(http.MethodPost, "/api/auth/logout", nil); status != http.StatusOK {
		t.Fatalf("sign out returned %d", status)
	}
	if status, _ := h.call(http.MethodGet, "/api/devices", nil); status != http.StatusUnauthorized {
		t.Fatalf("a signed-out session still works (status %d)", status)
	}
}

func TestWrongPasswordIsRefusedWithoutSayingWhy(t *testing.T) {
	h := newHarness(t)
	h.post("/api/auth/register", map[string]string{
		"email": "player@queueup.test", "password": "correct horse battery",
	})

	code, out := h.post("/api/auth/login", map[string]string{
		"email": "player@queueup.test", "password": "wrong",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	msg, _ := out["error"].(string)

	code2, out2 := h.post("/api/auth/login", map[string]string{
		"email": "nobody@queueup.test", "password": "wrong",
	})
	msg2, _ := out2["error"].(string)
	if code2 != http.StatusUnauthorized || msg != msg2 {
		t.Fatalf("an unknown email is distinguishable from a wrong password: %q vs %q", msg, msg2)
	}
}

func TestSearchAndFavourites(t *testing.T) {
	h := newHarness(t)

	status, out := h.call(http.MethodGet, "/api/servers/search?q=rust", nil)
	if status != http.StatusOK {
		t.Fatalf("search returned %d: %v", status, out)
	}
	list, _ := out["servers"].([]any)
	if len(list) == 0 {
		t.Fatal("search found nothing in the built-in list")
	}
	first := list[0].(map[string]any)
	id, _ := first["id"].(string)
	if id == "" {
		t.Fatalf("a search result has no id: %v", first)
	}
	if fav, _ := first["favourite"].(bool); fav {
		t.Error("a server is starred before anyone starred it")
	}

	if status, out := h.call(http.MethodPost, "/api/favourites", map[string]any{
		"server_id": id, "name": first["name"], "address": first["address"],
	}); status != http.StatusOK {
		t.Fatalf("saving a server returned %d: %v", status, out)
	}

	// The search results now say it is starred.
	_, out = h.call(http.MethodGet, "/api/servers/search?q=rust", nil)
	list, _ = out["servers"].([]any)
	if fav, _ := list[0].(map[string]any)["favourite"].(bool); !fav {
		t.Error("a saved server is not marked as saved in search results")
	}

	_, out = h.call(http.MethodGet, "/api/favourites", nil)
	favs, _ := out["favourites"].([]any)
	if len(favs) != 1 {
		t.Fatalf("saved servers = %d, want 1", len(favs))
	}

	if status, _ := h.call(http.MethodDelete, "/api/favourites/"+id, nil); status != http.StatusOK {
		t.Fatalf("removing a saved server failed")
	}
	_, out = h.call(http.MethodGet, "/api/favourites", nil)
	favs, _ = out["favourites"].([]any)
	if len(favs) != 0 {
		t.Fatalf("saved servers after removing = %d, want 0", len(favs))
	}
}

// The web app sends a server id, not an address. The relay looks the address up.
func TestJoinByServerIDResolvesTheAddress(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the job returned %d: %v", status, out)
	}
	if addr, _ := out["server_addr"].(string); addr != "51.83.128.10:28015" {
		t.Fatalf("server_addr = %q; the address was not looked up from the id", addr)
	}
	if name, _ := out["server_name"].(string); name != "Rustopia EU Main" {
		t.Errorf("server_name = %q; the name was not filled in from the lookup", name)
	}

	jobID := out["id"].(string)
	h.waitUntil("the join to finish", 20*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}

func TestUnknownServerIDIsRefusedInPlainLanguage(t *testing.T) {
	h := newHarness(t)
	deviceID, _ := h.pair()

	status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "no-such-server",
	})
	if status == http.StatusCreated {
		t.Fatal("a job was created for a server that does not exist")
	}
	if msg, _ := out["error"].(string); len(msg) < 15 {
		t.Errorf("refusal %q is too terse to show a user", msg)
	}
}

// movingServer is a provider whose server changes address between calls, which
// is what really happens to Rust servers between wipes.
type movingServer struct {
	calls int
}

func (m *movingServer) Name() string { return "moving" }

func (m *movingServer) Search(context.Context, string, int) ([]servers.Server, error) {
	s, _ := m.ByID(context.Background(), "moving-1")
	return []servers.Server{s}, nil
}

func (m *movingServer) ByID(context.Context, string) (servers.Server, error) {
	m.calls++
	addr := "10.0.0.1:28015"
	if m.calls > 1 {
		addr = "10.0.0.99:28015" // it moved
	}
	return servers.Server{
		ID: "moving-1", Name: "Wandering Server", Address: addr,
		Online: true, Players: 10, MaxPlayers: 200,
	}, nil
}

// The address is looked up again just before the job is handed to the PC, so a
// server that moved between the job being created and it being run is still
// joined correctly.
func TestAddressIsRefreshedWhenTheJobIsHandedOver(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, token, err := st.CreateAccount("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := &movingServer{}
	srv := relay.New(relay.Config{Store: st, Log: quiet, Servers: provider})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	h := &harness{t: t, st: st, srv: srv, http: ts, acctToken: token, provider: provider}
	deviceID, deviceToken := h.pair()

	// Create the job while the PC is off, so it is dispatched later.
	status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "moving-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the job returned %d: %v", status, out)
	}
	jobID := out["id"].(string)
	if addr, _ := out["server_addr"].(string); addr != "10.0.0.1:28015" {
		t.Fatalf("first address = %q, want 10.0.0.1:28015", addr)
	}

	// Now the PC comes online and the job is handed over. The address must be
	// looked up again at that moment.
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()

	h.waitUntil("the job to pick up the new address", 15*time.Second, func() bool {
		j, err := h.st.JobByID(jobID)
		return err == nil && j.ServerAddr == "10.0.0.99:28015"
	})

	var toldTheUser bool
	for _, line := range h.timeline(jobID) {
		if contains(line, "changed address") {
			toldTheUser = true
		}
	}
	if !toldTheUser {
		t.Errorf("the timeline never mentions the address change:\n%v", h.timeline(jobID))
	}
}

// Checking a password is deliberately expensive, which is what makes a stolen
// password file useless. It also means unlimited sign-in attempts are a way to
// eat the relay's processor, and the relay is one small machine that everybody's
// join depends on. So attempts are capped.
func TestRepeatedWrongPasswordsAreEventuallyRefused(t *testing.T) {
	h := newHarness(t)
	h.post("/api/auth/register", map[string]string{
		"email": "target@queueup.test", "password": "correct horse battery",
	})

	var blocked bool
	for i := 0; i < 12; i++ {
		code, out := h.post("/api/auth/login", map[string]string{
			"email": "target@queueup.test", "password": "wrong guess",
		})
		if code == http.StatusTooManyRequests {
			blocked = true
			if msg, _ := out["error"].(string); len(msg) < 20 {
				t.Errorf("refusal %q is too terse to show a user", msg)
			}
			break
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d: %v", i+1, code, out)
		}
	}
	if !blocked {
		t.Fatal("twelve wrong passwords in a row and it was still happily checking more")
	}
}

// The browser shows the busiest servers first: full servers with queues are
// what people came looking for, and a random 25 reads as noise.
func TestSearchReturnsBusiestFirst(t *testing.T) {
	h := newHarness(t)
	status, out := h.call(http.MethodGet, "/api/servers/search?q=", nil)
	if status != http.StatusOK {
		t.Fatalf("search returned %d: %v", status, out)
	}
	list, _ := out["servers"].([]any)
	if len(list) < 3 {
		t.Fatalf("only %d servers", len(list))
	}
	prevBusy := 1 << 30
	for i, item := range list {
		sv := item.(map[string]any)
		online, _ := sv["online"].(bool)
		players := int(sv["players"].(float64))
		queue := int(sv["queue"].(float64))
		busy := players + queue*10000
		if !online {
			busy = -1
		}
		if busy > prevBusy {
			t.Fatalf("entry %d (%v) is busier than the one before it; the list is not sorted", i, sv["name"])
		}
		prevBusy = busy
	}
}
