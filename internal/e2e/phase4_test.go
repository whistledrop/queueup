package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"queueup/internal/a2s"
	"queueup/internal/notify"
	"queueup/internal/relay"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// inbox collects everything the user's phone would have been told.
type inbox struct {
	mu   sync.Mutex
	msgs []notify.Message
}

func (i *inbox) add(_ string, m notify.Message) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.msgs = append(i.msgs, m)
}

func (i *inbox) titles() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []string
	for _, m := range i.msgs {
		out = append(out, m.Title)
	}
	return out
}

func (i *inbox) has(title string) bool {
	for _, t := range i.titles() {
		if t == title {
			return true
		}
	}
	return false
}

func (i *inbox) waitFor(t *testing.T, title string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if i.has(title) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no notification titled %q arrived; inbox: %v", title, i.titles())
}

// phase4Harness runs the full relay: scheduler and server watcher included,
// notifications captured.
func phase4Harness(t *testing.T) (*harness, *inbox) {
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
	box := &inbox{}
	notifier := &notify.Notifier{Store: st, Log: quiet, SendHook: box.add}
	provider := servers.NewStub()
	srv := relay.New(relay.Config{
		Store: st, Log: quiet, AdminToken: "admin-test-token",
		Servers: provider, Notifier: notifier,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.RunScheduler(ctx, 100*time.Millisecond)

	return &harness{t: t, st: st, srv: srv, http: ts, acctToken: token, provider: provider}, box
}

// The headline schedule case, with the timezone baked in: the phone is in
// Spain (+02:00), the relay stores UTC, and the join fires at the right moment.
func TestScheduledJoinFiresOnTimeAndRuns(t *testing.T) {
	h, box := phase4Harness(t)
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

	box.waitFor(t, "Scheduled join started", 10*time.Second)

	sc, err := h.st.ScheduleByID(schedID)
	if err != nil || sc.State != "fired" || sc.JobID == "" {
		t.Fatalf("schedule after firing = %+v, %v", sc, err)
	}
	h.waitUntil("the scheduled join to finish", 20*time.Second, func() bool {
		return h.jobState(sc.JobID) == "done"
	})
}

// A schedule that fires while the PC is off must not be lost, and the user must
// hear about it immediately, while there may still be time to fix the PC.
func TestScheduleFiringIntoAnOfflinePC(t *testing.T) {
	h, box := phase4Harness(t)
	deviceID, deviceToken := h.pair()

	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(500 * time.Millisecond).UTC().Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the schedule returned %d: %v", status, out)
	}

	box.waitFor(t, "Your PC is offline", 10*time.Second)

	// The job exists and is waiting. Switch the PC on: it must pick it up.
	j, err := h.st.ActiveJobForDevice(deviceID)
	if err != nil {
		t.Fatal("the job was lost because the PC was off")
	}
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the handed-over join to finish", 20*time.Second, func() bool {
		return h.jobState(j.ID) == "done"
	})
}

func TestCancellingASchedule(t *testing.T) {
	h, _ := phase4Harness(t)
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
	h, _ := phase4Harness(t)

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

// The notifications a queue day actually produces, in order.
func TestQueueNotifications(t *testing.T) {
	h, box := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	h.createJob(deviceID, "51.83.128.10:28015")

	box.waitFor(t, "In the queue", 15*time.Second) // entered at 212
	box.waitFor(t, "Position 61", 15*time.Second)  // crossed the 100 line
	box.waitFor(t, "Position 12", 15*time.Second)  // crossed the 50 line
	box.waitFor(t, "Position 1", 15*time.Second)   // crossed the 10 line
	box.waitFor(t, "You're in", 15*time.Second)

	// Every position change on the phone would be spam; only the entry, the
	// milestone crossings and the arrival may notify. Position 148 in
	// particular must NOT have buzzed: it crossed no line.
	allowed := map[string]bool{
		"In the queue": true, "Position 61": true, "Position 12": true,
		"Position 1": true, "You're in": true,
	}
	count := 0
	for _, title := range box.titles() {
		if allowed[title] {
			count++
		} else {
			t.Errorf("unexpected notification %q", title)
		}
	}
	if count != 5 {
		t.Errorf("got %d notifications, want exactly 5: %v", count, box.titles())
	}
}

func TestFailureNotificationSpeaksPlainly(t *testing.T) {
	h, box := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "steam_not_logged_in")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})
	h.createJob(deviceID, "51.83.128.10:28015")

	box.waitFor(t, "Join didn't work", 15*time.Second)
	found := false
	box.mu.Lock()
	for _, m := range box.msgs {
		if m.Title == "Join didn't work" && contains(m.Body, "Steam isn't logged in") {
			found = true
		}
	}
	box.mu.Unlock()
	if !found {
		t.Error("the failure notification does not carry the plain-language reason")
	}
}

// A reboot mid-queue should tell the user twice: gone, and back.
func TestOfflineAndBackNotifications(t *testing.T) {
	h, box := phase4Harness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})
	jobID := h.createJob(deviceID, "51.83.128.10:28015")
	h.waitUntil("the job to reach the queue", 10*time.Second, func() bool {
		return h.jobState(jobID) == "queued"
	})

	stop() // the PC "reboots"
	box.waitFor(t, "Your PC went offline", 10*time.Second)

	_, stop2 := h.agent(deviceToken, "long_queue")
	defer stop2()
	box.waitFor(t, "Your PC is back online", 10*time.Second)

	h.waitUntil("the join to finish after the reboot", 25*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}
