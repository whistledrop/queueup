package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"queueup/internal/a2s"
	"queueup/internal/relay"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// phase4Harness runs the full relay: scheduler and server watcher included,
// notifications captured.
func phase4Harness(t *testing.T) *harness {
	t.Helper()
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
	provider := servers.NewStub()
	srv := relay.New(relay.Config{
		Store: st, Log: quiet, AdminToken: "admin-test-token",
		Servers: provider,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.RunScheduler(ctx, 100*time.Millisecond)

	return &harness{t: t, st: st, srv: srv, http: ts, acctToken: token, provider: provider}
}

// The headline schedule case, with the timezone baked in: the phone is in
// Spain (+02:00), the relay stores UTC, and the join fires at the right moment.
func TestScheduledJoinFiresOnTimeAndRuns(t *testing.T) {
	h := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	// A few seconds from now, written the way a phone in Madrid would write it.
	const fireDelay = 3 * time.Second
	armed := time.Now()
	fireAt := armed.Add(fireDelay).In(time.FixedZone("CEST", 2*3600))
	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": fireAt.Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the schedule returned %d: %v", status, out)
	}
	schedID := out["id"].(string)

	// Nothing may happen before the time comes. Under the race detector on a
	// loaded machine the clock really can pass fireAt before this line runs, so
	// the assertion only counts when we are genuinely still early.
	time.Sleep(300 * time.Millisecond)
	if time.Since(armed) < fireDelay-time.Second {
		if _, err := h.st.ActiveJobForDevice(deviceID); err == nil {
			t.Fatal("the join started before its time")
		}
	}

	h.waitUntil("the schedule to fire", 10*time.Second, func() bool {
		got, err := h.st.ScheduleByID(schedID)
		return err == nil && got.State == "fired" && got.JobID != ""
	})
	sc, err := h.st.ScheduleByID(schedID)
	if err != nil {
		t.Fatal(err)
	}
	h.waitUntil("the scheduled join to finish", 20*time.Second, func() bool {
		return h.jobState(sc.JobID) == "done"
	})
}

// A schedule that fires while the PC is off must not be lost: the job is
// created, the timeline says the PC is off, and the PC picks it up on return.
func TestScheduleFiringIntoAnOfflinePC(t *testing.T) {
	h := phase4Harness(t)
	deviceID, deviceToken := h.pair()

	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(500 * time.Millisecond).UTC().Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the schedule returned %d: %v", status, out)
	}

	h.waitUntil("the schedule to fire into a job", 10*time.Second, func() bool {
		_, err := h.st.ActiveJobForDevice(deviceID)
		return err == nil
	})
	j, err := h.st.ActiveJobForDevice(deviceID)
	if err != nil {
		t.Fatal("the job was lost because the PC was off")
	}
	var toldOffline bool
	for _, line := range h.timeline(j.ID) {
		if contains(line, "offline") {
			toldOffline = true
		}
	}
	if !toldOffline {
		t.Errorf("nothing in the timeline says the PC was off:\n%v", h.timeline(j.ID))
	}
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the handed-over join to finish", 20*time.Second, func() bool {
		return h.jobState(j.ID) == "done"
	})
}

func TestCancellingASchedule(t *testing.T) {
	h := phase4Harness(t)
	deviceID, _ := h.pair()

	_, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	schedID := out["id"].(string)

	if status, body := h.call(http.MethodPost, "/api/schedules/"+schedID+"/cancel", nil); status != http.StatusOK {
		t.Fatalf("cancel returned %d: %v", status, body)
	}
	// Long after its time would have come, nothing has fired.
	time.Sleep(400 * time.Millisecond)
	if _, err := h.st.ActiveJobForDevice(deviceID); err == nil {
		t.Fatal("a cancelled schedule still fired")
	}
}

// The wipe-day headline: a real (fake) game server goes down and comes back,
// the watcher spots it over real UDP within seconds, and the agent connects.
func TestWipeRestartIsDetectedAndTheAgentConnects(t *testing.T) {
	h := phase4Harness(t)

	game, err := a2s.NewFakeServer(a2s.Info{
		Name: "Wipe Test", Players: 0, MaxPlayers: 200,
	}, false) // starts DOWN, mid-wipe
	if err != nil {
		t.Fatal(err)
	}
	defer game.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher := &relay.Watcher{
		Store: h.st, Hub: h.srv.Hub(),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		WaitingPoll: 100 * time.Millisecond,
	}
	go watcher.Run(ctx)

	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	// Arm the join against the fake game server's real UDP address.
	status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server": game.Addr(), "server_name": "Wipe Test",
		"wait_for_server_up": true,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the job returned %d: %v", status, out)
	}
	jobID := out["id"].(string)

	// While the server is down, the agent must hold and not launch.
	h.waitUntil("the job to be waiting for the server", 10*time.Second, func() bool {
		return h.jobState(jobID) == "waiting_for_server_up"
	})
	time.Sleep(600 * time.Millisecond)
	if got := h.jobState(jobID); got != "waiting_for_server_up" {
		t.Fatalf("job moved to %q while the server was still down", got)
	}

	// The wipe restart finishes: the server answers queries again.
	game.SetOnline(true)

	h.waitUntil("the join to complete after the server came back", 25*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})

	var sawBackUp bool
	for _, line := range h.timeline(jobID) {
		if contains(line, "back up") {
			sawBackUp = true
		}
	}
	if !sawBackUp {
		t.Errorf("the timeline never says the server came back:\n%v", h.timeline(jobID))
	}
}

// One PC cannot be in two places. A join started now occupies the machine, and
// if it is still running when the schedule fires, the scheduled join is lost:
// the wipe, silently missed, because of a casual join hours earlier. So a
// pending schedule blocks an immediate join, and says how to proceed.
func TestJoiningNowIsRefusedWhileAJoinIsScheduled(t *testing.T) {
	h := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the schedule returned %d: %v", status, out)
	}
	schedID := out["id"].(string)

	status, out = h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-2",
	})
	if status != http.StatusConflict {
		t.Fatalf("joining now returned %d, want 409 while a join is scheduled", status)
	}
	msg, _ := out["error"].(string)
	for _, want := range []string{"scheduled", "cancel"} {
		if !contains(strings.ToLower(msg), want) {
			t.Errorf("refusal %q does not mention %q, so the player cannot act on it", msg, want)
		}
	}

	// Cancelling the schedule frees the PC immediately.
	if status, _ := h.call(http.MethodPost, "/api/schedules/"+schedID+"/cancel", nil); status != http.StatusOK {
		t.Fatalf("cancelling the schedule failed")
	}
	if status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-2",
	}); status != http.StatusCreated {
		t.Fatalf("joining after cancelling the schedule returned %d: %v", status, out)
	}
}

// A schedule set days ahead is legitimate even while the PC is mid-join today:
// the current join will be long over. What must never happen is the one Logan
// hit from the other side, where the casual join is still running at fire time
// and the WIPE join is the one that loses. The planned join wins, because
// somebody deliberately set it and nobody deliberately set the other one to
// still be running.
func TestAScheduledJoinTakesOverFromAJoinStillRunning(t *testing.T) {
	h := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue_slow")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	// A casual join, started before any schedule existed, and still queueing.
	casual := h.createJob(deviceID, "51.83.128.10:28015")
	h.waitUntil("the casual join to be queueing", 10*time.Second, func() bool {
		return h.jobState(casual) == "queued"
	})

	// A schedule fires while it is still running.
	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-2",
		"fire_at": time.Now().Add(500 * time.Millisecond).UTC().Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the schedule returned %d: %v", status, out)
	}
	schedID := out["id"].(string)

	// The schedule must run, not fail.
	h.waitUntil("the schedule to fire and take over", 15*time.Second, func() bool {
		sc, err := h.st.ScheduleByID(schedID)
		return err == nil && sc.State == "fired" && sc.JobID != ""
	})

	// And the casual join must be closed out, not left half-alive.
	h.waitUntil("the casual join to be stood down", 10*time.Second, func() bool {
		return h.jobState(casual) == "done" || h.jobState(casual) == "failed"
	})

	var toldWhy bool
	for _, line := range h.timeline(casual) {
		if contains(strings.ToLower(line), "scheduled") {
			toldWhy = true
		}
	}
	if !toldWhy {
		t.Errorf("the casual join was stopped without saying why:\n%v", h.timeline(casual))
	}
}
