// Package e2e runs the whole system end to end: a real relay over real HTTP, a
// real agent holding a real WebSocket open, and the fake Rust client at the far
// end. No game, no Windows PC, no wipe day required.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"queueup/internal/agentapp"
	"queueup/internal/game"
	"queueup/internal/job"
	"queueup/internal/logparse"
	"queueup/internal/protocol"
	"queueup/internal/relay"
	"queueup/internal/relayclient"
	"queueup/internal/scenario"
	"queueup/internal/servers"
	"queueup/internal/store"
)

const simSpeed = 8

type harness struct {
	t         *testing.T
	st        *store.Store
	srv       *relay.Server
	http      *httptest.Server
	acctToken string
	provider  servers.Provider
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	_, token, err := st.CreateAccount("player@example.com")
	if err != nil {
		t.Fatalf("creating account: %v", err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := servers.NewStub()
	srv := relay.New(relay.Config{
		Store: st, Log: quiet, AdminToken: "admin-test-token", Servers: provider,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &harness{t: t, st: st, srv: srv, http: ts, acctToken: token, provider: provider}
}

// call makes an authenticated API request, the way the web app will.
func (h *harness) call(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.http.URL+path, rdr)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.acctToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.http.Client().Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// pair runs the whole pairing handshake: the PC asks for a code, the user types
// it into the web app, the PC collects its token.
func (h *harness) pair() (deviceID, deviceToken string) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start, err := relayclient.StartPairing(ctx, h.http.URL, "Test PC")
	if err != nil {
		h.t.Fatalf("StartPairing: %v", err)
	}
	if len(start.Code) != 6 {
		h.t.Fatalf("pairing code %q should be six characters", start.Code)
	}

	code, ok := h.call(http.MethodPost, "/api/pair", map[string]string{"code": start.Code})
	if code != http.StatusOK {
		h.t.Fatalf("claiming the code failed: %d %v", code, ok)
	}

	token, err := relayclient.WaitForPairing(ctx, h.http.URL, start.ClaimToken, 20*time.Millisecond)
	if err != nil {
		h.t.Fatalf("WaitForPairing: %v", err)
	}
	return start.DeviceID, token
}

// agent starts an agent connected to the relay, running the given scenario. The
// returned stop function is a clean shutdown; killAgent below is the rude one.
func (h *harness) agent(deviceToken, scenarioName string) (*agentapp.App, func()) {
	h.t.Helper()

	sc, err := scenario.Load(filepath.Join("../../testdata/scenarios", scenarioName+".json"))
	if err != nil {
		h.t.Fatalf("loading scenario: %v", err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		h.t.Fatalf("loading patterns: %v", err)
	}
	logDir := h.t.TempDir()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	client := &relayclient.Client{
		RelayURL: h.http.URL, DeviceToken: deviceToken,
		AgentVersion: "test", OS: "test", Hostname: "test-pc", Simulator: true,
		Log: quiet, MaxBackoff: 200 * time.Millisecond,
	}
	app := &agentapp.App{
		Client: client, Parser: parser, Log: quiet,
		NewGame: func(j protocol.Job) (game.Launcher, error) {
			return &game.SimLauncher{
				Scenario: sc,
				Log:      filepath.Join(logDir, j.ID+"-Player.log"),
				Speed:    simSpeed,
			}, nil
		},
		JobConfig: job.Config{
			InServerConfirm: 300 * time.Millisecond,
			RetryBase:       200 * time.Millisecond,
			RetryMax:        200 * time.Millisecond,
			MaxAttempts:     3,
		},
	}
	client.Handler = app

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.Run(ctx)
	}()

	return app, func() {
		cancel()
		app.Stop()
		<-done
	}
}

func (h *harness) waitUntil(what string, timeout time.Duration, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

func (h *harness) jobState(jobID string) string {
	j, err := h.st.JobByID(jobID)
	if err != nil {
		return ""
	}
	return j.State
}

func (h *harness) createJob(deviceID, server string) string {
	h.t.Helper()
	code, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server": server,
	})
	if code != http.StatusCreated {
		h.t.Fatalf("creating the job failed: %d %v", code, out)
	}
	id, _ := out["id"].(string)
	if id == "" {
		h.t.Fatalf("no job id came back: %v", out)
	}
	return id
}

func (h *harness) timeline(jobID string) []string {
	events, err := h.st.Events(jobID, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, e := range events {
		out = append(out, e.State+": "+e.Detail)
	}
	return out
}

// ------------------------------------------------------------------ tests

// The headline path: pair a PC, send it a join from the web app, watch it work.
func TestPairThenJoinThroughTheRelay(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()

	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()

	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	jobID := h.createJob(deviceID, "51.83.128.10:28015")

	h.waitUntil("the join to finish", 20*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})

	j, err := h.st.JobByID(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != "done" {
		t.Fatalf("job state = %s, want done. timeline:\n%v", j.State, h.timeline(jobID))
	}

	// The queue positions must have made it all the way up to the relay, since
	// that is what the phone shows.
	events, _ := h.st.Events(jobID, 0)
	var positions []int
	for _, e := range events {
		if e.State == "queued" {
			positions = append(positions, e.Position)
		}
	}
	want := []int{212, 148, 61, 12, 1}
	if len(positions) != len(want) {
		t.Fatalf("relay recorded positions %v, want %v", positions, want)
	}
	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf("relay recorded positions %v, want %v", positions, want)
		}
	}
}

// The resilience case the whole product hangs on: the PC disappears mid-queue,
// as it would during a forced Windows update, and comes back on its own.
func TestJobResumesAfterTheAgentRestarts(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()

	_, stop := h.agent(deviceToken, "long_queue")
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	jobID := h.createJob(deviceID, "51.83.128.10:28015")

	h.waitUntil("the job to reach the queue", 10*time.Second, func() bool {
		return h.jobState(jobID) == "queued"
	})

	// The PC reboots. Nothing tidy happens: the socket just stops.
	stop()
	h.waitUntil("the relay to notice the PC has gone", 5*time.Second, func() bool {
		return !h.srv.Hub().Online(deviceID)
	})

	// The job is still there, and still active. This is the point: the relay is
	// the source of truth, and the PC had to remember nothing.
	j, err := h.st.JobByID(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !j.Active() {
		t.Fatalf("the job was lost when the PC went away: state = %s", j.State)
	}

	// Windows comes back, the agent auto-starts, and reconnects.
	_, stop2 := h.agent(deviceToken, "long_queue")
	defer stop2()

	h.waitUntil("the join to finish after the reboot", 25*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})

	timeline := h.timeline(jobID)
	var sawOffline, sawResume bool
	for _, line := range timeline {
		if contains(line, "went offline") {
			sawOffline = true
		}
		if contains(line, "picking this join back up") {
			sawResume = true
		}
	}
	if !sawOffline {
		t.Errorf("the timeline never mentions the PC going offline:\n%v", timeline)
	}
	if !sawResume {
		t.Errorf("the timeline never mentions the job resuming:\n%v", timeline)
	}
}

// A job created while the PC is switched off must wait, not fail.
func TestJobWaitsForAnOfflinePC(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()

	jobID := h.createJob(deviceID, "51.83.128.10:28015")
	if got := h.jobState(jobID); got != "pending" {
		t.Fatalf("job state = %s, want pending while the PC is off", got)
	}

	var sawOfflineNote bool
	for _, line := range h.timeline(jobID) {
		if contains(line, "offline") {
			sawOfflineNote = true
		}
	}
	if !sawOfflineNote {
		t.Errorf("nothing in the timeline explains that the PC is off:\n%v", h.timeline(jobID))
	}

	// Now the PC is switched on.
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()

	h.waitUntil("the waiting job to run and finish", 20*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}

func TestCancelFromTheWebAppStopsTheJob(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()

	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})
	jobID := h.createJob(deviceID, "51.83.128.10:28015")
	h.waitUntil("the job to reach the queue", 10*time.Second, func() bool {
		return h.jobState(jobID) == "queued"
	})

	code, out := h.call(http.MethodPost, "/api/jobs/"+jobID+"/cancel", nil)
	if code != http.StatusOK {
		t.Fatalf("cancel failed: %d %v", code, out)
	}
	h.waitUntil("the job to stop", 10*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}

// One PC, one job. A second job would be two things fighting over one copy of
// Rust.
func TestASecondJobForTheSamePCIsRefused(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	h.createJob(deviceID, "51.83.128.10:28015")
	code, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server": "51.83.128.11:28015",
	})
	if code != http.StatusConflict {
		t.Fatalf("second job returned %d, want 409. body: %v", code, out)
	}
	if msg, _ := out["error"].(string); len(msg) < 20 {
		t.Errorf("refusal message %q is too terse to show a user", msg)
	}
}

// Unlinking a PC has to take effect immediately, not whenever the socket next
// happens to break.
func TestUnlinkingAPCDisconnectsItImmediately(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()

	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	code, out := h.call(http.MethodPost, "/api/devices/"+deviceID+"/revoke", nil)
	if code != http.StatusOK {
		t.Fatalf("revoke failed: %d %v", code, out)
	}
	h.waitUntil("the PC to be disconnected", 5*time.Second, func() bool {
		return !h.srv.Hub().Online(deviceID)
	})

	// And it must not be able to get back in.
	time.Sleep(500 * time.Millisecond)
	if h.srv.Hub().Online(deviceID) {
		t.Fatal("an unlinked PC reconnected")
	}
}

// One account must never be able to touch another account's PC.
func TestAnotherAccountCannotCommandThisPC(t *testing.T) {
	h := newHarness(t)
	deviceID, _ := h.pair()

	_, intruderToken, err := h.st.CreateAccount("intruder@example.com")
	if err != nil {
		t.Fatal(err)
	}
	saved := h.acctToken
	h.acctToken = intruderToken
	defer func() { h.acctToken = saved }()

	code, _ := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server": "51.83.128.10:28015",
	})
	if code != http.StatusNotFound {
		t.Fatalf("another account got %d when commanding this PC, want 404", code)
	}
}

func TestUnauthenticatedRequestsAreRefused(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.http.URL + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdminViewNeedsItsToken(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	resp, err := http.Get(h.http.URL + "/admin/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin view without a token = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, h.http.URL+"/admin/status", nil)
	req.Header.Set("Authorization", "Bearer admin-test-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&out)
	if n, _ := out["connected_agents"].(float64); int(n) != 1 {
		t.Fatalf("admin view reports %v connected agents, want 1", out["connected_agents"])
	}
}

// The live status stream is what the phone will listen to.
func TestStatusStreamsToAWatchingClient(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})
	jobID := h.createJob(deviceID, "51.83.128.10:28015")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		h.http.URL+"/api/jobs/"+jobID+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+h.acctToken)

	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", ct)
	}

	seen := map[string]bool{}
	buf := make([]byte, 4096)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && !seen["done"] {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			for _, state := range []string{"launching", "queued", "in_server", "done"} {
				if contains(string(buf[:n]), `"state":"`+state+`"`) {
					seen[state] = true
				}
			}
		}
		if err != nil {
			break
		}
	}
	for _, state := range []string{"launching", "queued", "done"} {
		if !seen[state] {
			t.Errorf("the status stream never reported %q; saw %v", state, seen)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && bytes.Contains([]byte(s), []byte(sub)))
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var _ = fmt.Sprintf
