package e2e

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"queueup/internal/notify"
	"queueup/internal/relay"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// The gate: setting up is free, joining is the product. These tests run the
// relay with billing ON, which is the state once Stripe is connected.

func billingHarness(t *testing.T) *harness {
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
	srv := relay.New(relay.Config{
		Store: st, Log: quiet, Servers: servers.NewStub(),
		Notifier:       &notify.Notifier{Store: st, Log: quiet, SendHook: func(string, notify.Message) {}},
		BillingEnabled: true,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &harness{t: t, st: st, srv: srv, http: ts, acctToken: token}
}

func (h *harness) account(t *testing.T) store.Account {
	t.Helper()
	acct, err := h.st.AccountByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return acct
}

// Everything up to and including pairing is free. The Join button is not.
func TestSetupIsFreeButJoiningIsGated(t *testing.T) {
	h := billingHarness(t)

	// Pairing works with no subscription: that is the whole point of the flow.
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	// Joining is refused with 402 and a message a person can read.
	status, out := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
	})
	if status != http.StatusPaymentRequired {
		t.Fatalf("join without subscribing returned %d, want 402: %v", status, out)
	}
	msg, _ := out["error"].(string)
	if len(msg) < 30 || !contains(msg, "free") {
		t.Errorf("the refusal %q should say the price and that setup was free", msg)
	}

	// Nothing was created.
	if _, err := h.st.ActiveJobForDevice(deviceID); err == nil {
		t.Fatal("a job was created despite the gate")
	}

	// Subscribe (the way the Stripe webhook will), and the same tap works.
	if err := h.st.SetSubscription(h.account(t).ID, "active", "sub_test"); err != nil {
		t.Fatal(err)
	}
	status, out = h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("join after subscribing returned %d: %v", status, out)
	}
	jobID := out["id"].(string)
	h.waitUntil("the join to finish", 20*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}

func TestSchedulingIsGatedTheSameWay(t *testing.T) {
	h := billingHarness(t)
	deviceID, _ := h.pair()

	status, out := h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	if status != http.StatusPaymentRequired {
		t.Fatalf("schedule without subscribing returned %d, want 402: %v", status, out)
	}

	if err := h.st.SetSubscription(h.account(t).ID, "active", "sub_test"); err != nil {
		t.Fatal(err)
	}
	status, _ = h.call(http.MethodPost, "/api/schedules", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
		"fire_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	if status != http.StatusCreated {
		t.Fatalf("schedule after subscribing returned %d", status)
	}
}

// Cancelling a running join, watching status, unlinking the PC: never gated.
// Someone whose card expires mid-queue must still be able to stop their PC.
func TestControlOfARunningJobIsNeverGated(t *testing.T) {
	h := billingHarness(t)
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	if err := h.st.SetSubscription(h.account(t).ID, "active", "sub_test"); err != nil {
		t.Fatal(err)
	}
	jobID := h.createJob(deviceID, "51.83.128.10:28015")
	h.waitUntil("the job to reach the queue", 10*time.Second, func() bool {
		return h.jobState(jobID) == "queued"
	})

	// The subscription lapses mid-queue.
	if err := h.st.SetSubscription(h.account(t).ID, "none", ""); err != nil {
		t.Fatal(err)
	}

	// They can still see it and still stop it.
	if status, _ := h.call(http.MethodGet, "/api/jobs/"+jobID, nil); status != http.StatusOK {
		t.Fatalf("watching a running job was gated (status %d)", status)
	}
	if status, _ := h.call(http.MethodPost, "/api/jobs/"+jobID+"/cancel", nil); status != http.StatusOK {
		t.Fatalf("cancelling was gated (status %d)", status)
	}
	if status, _ := h.call(http.MethodPost, "/api/devices/"+deviceID+"/revoke", nil); status != http.StatusOK {
		t.Fatalf("unlinking the PC was gated (status %d)", status)
	}
}

func TestBillingStateIsReported(t *testing.T) {
	h := billingHarness(t)

	status, out := h.call(http.MethodGet, "/api/billing", nil)
	if status != http.StatusOK {
		t.Fatalf("billing returned %d", status)
	}
	if enabled, _ := out["enabled"].(bool); !enabled {
		t.Error("billing should report enabled")
	}
	if subscribed, _ := out["subscribed"].(bool); subscribed {
		t.Error("a fresh account should not report subscribed")
	}

	// The checkout placeholder answers honestly instead of erroring vaguely.
	status, out = h.call(http.MethodPost, "/api/billing/checkout", nil)
	if status != http.StatusNotImplemented {
		t.Fatalf("checkout placeholder returned %d: %v", status, out)
	}

	if err := h.st.SetSubscription(h.account(t).ID, "active", "sub_test"); err != nil {
		t.Fatal(err)
	}
	_, out = h.call(http.MethodGet, "/api/billing", nil)
	if subscribed, _ := out["subscribed"].(bool); !subscribed {
		t.Error("after subscribing, billing should report subscribed")
	}
}

// With billing off (the state until Stripe is connected), nothing is gated and
// the API says everyone is effectively subscribed, so the web app never shows
// a paywall it cannot honour.
func TestBillingOffMeansEverythingIsFree(t *testing.T) {
	h := newHarness(t) // the ordinary harness: BillingEnabled false
	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "instant_join")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	_, out := h.call(http.MethodGet, "/api/billing", nil)
	if subscribed, _ := out["subscribed"].(bool); !subscribed {
		t.Error("with billing off, subscribed should read true so no paywall shows")
	}

	status, _ := h.call(http.MethodPost, "/api/jobs", map[string]any{
		"device_id": deviceID, "server_id": "stub-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("with billing off, a join returned %d", status)
	}
}
