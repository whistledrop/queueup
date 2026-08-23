package e2e

import (
	"io"
	"log/slog"
	"net"
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

// The relay itself dies and comes back mid-queue. The agent must ride it out:
// keep the game queuing locally, reconnect on its own, and let the relay catch
// back up. Jobs are in SQLite, so the restarted relay remembers everything.
func TestAgentSurvivesARelayOutageMidQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "relay.db")
	st, err := store.Open(dbPath)
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

	// A fixed port, so the restarted relay comes back at the SAME address the
	// agent keeps trying. That is exactly what a redeployed relay looks like.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	newRelay := func() *relay.Server {
		return relay.New(relay.Config{
			Store: st, Log: quiet, Servers: servers.NewStub(),
			Notifier: &notify.Notifier{Store: st, Log: quiet, SendHook: box.add},
		})
	}

	srv1 := newRelay()
	ts1 := &httptest.Server{Listener: ln, Config: &http.Server{Handler: srv1}}
	ts1.Start()

	h := &harness{t: t, st: st, srv: srv1, http: ts1, acctToken: token}

	deviceID, deviceToken := h.pair()
	_, stop := h.agent(deviceToken, "long_queue_slow")
	defer stop()
	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return srv1.Hub().Online(deviceID)
	})

	jobID := h.createJob(deviceID, "51.83.128.10:28015")
	h.waitUntil("the job to reach the queue", 10*time.Second, func() bool {
		return h.jobState(jobID) == "queued"
	})

	// The relay goes down hard. Killing the process severs every socket, which
	// is what the agent actually experiences; httptest cannot close hijacked
	// websocket connections itself, so the hub drops them explicitly.
	srv1.Hub().DisconnectAll()
	ts1.Close()

	// While it is down, the agent's game session carries on queuing locally.
	time.Sleep(1 * time.Second)

	// The relay is redeployed at the same address, same database.
	var ln2 net.Listener
	deadline := time.Now().Add(5 * time.Second)
	for {
		ln2, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rebinding %s: %v", addr, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	srv2 := newRelay()
	ts2 := &httptest.Server{Listener: ln2, Config: &http.Server{Handler: srv2}}
	ts2.Start()
	t.Cleanup(ts2.Close)
	h.srv = srv2
	h.http = ts2

	// The agent must find its own way back...
	h.waitUntil("the agent to reconnect to the restarted relay", 15*time.Second, func() bool {
		return srv2.Hub().Online(deviceID)
	})

	// ...and the job, which never stopped on the PC, must finish and be
	// recorded by the restarted relay.
	h.waitUntil("the job to finish after the outage", 30*time.Second, func() bool {
		return h.jobState(jobID) == "done"
	})
}
