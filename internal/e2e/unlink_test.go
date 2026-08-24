package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"queueup/internal/agentapp"
	"queueup/internal/agentcfg"
	"queueup/internal/game"
	"queueup/internal/logparse"
	"queueup/internal/protocol"
	"queueup/internal/relayclient"
	"queueup/internal/scenario"
)

// Unlinking a PC from the web app must leave that PC able to pair again.
// Without this, the agent keeps a token the relay will never accept, retries it
// forever, and the only way back is finding and deleting a settings file: a
// dead end for anyone who is not a developer.
func TestUnlinkedAgentForgetsItsTokenSoItCanPairAgain(t *testing.T) {
	h := newHarness(t)
	deviceID, deviceToken := h.pair()

	// A settings file, as the real agent keeps one.
	cfgPath := filepath.Join(t.TempDir(), "agent.json")
	if err := agentcfg.Save(cfgPath, agentcfg.Config{
		RelayURL: h.http.URL, DeviceToken: deviceToken, DeviceID: deviceID,
	}); err != nil {
		t.Fatal(err)
	}

	sc, err := scenario.Load("../../testdata/scenarios/instant_join.json")
	if err != nil {
		t.Fatal(err)
	}
	parser, err := logparse.Load("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	logDir := t.TempDir()

	client := &relayclient.Client{
		RelayURL: h.http.URL, DeviceToken: deviceToken,
		AgentVersion: "test", OS: "test", Hostname: "test-pc", Simulator: true,
		Log: quiet, MaxBackoff: 100 * time.Millisecond,
	}
	app := &agentapp.App{
		Client: client, Parser: parser, Log: quiet,
		NewGame: func(j protocol.Job) (game.Launcher, error) {
			return &game.SimLauncher{Scenario: sc, Log: filepath.Join(logDir, j.ID+".log"), Speed: 20}, nil
		},
	}
	client.Handler = app

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx) }()

	h.waitUntil("the PC to connect", 5*time.Second, func() bool {
		return h.srv.Hub().Online(deviceID)
	})

	// The user taps "unlink this PC" on their phone.
	if status, _ := h.call(http.MethodPost, "/api/devices/"+deviceID+"/revoke", nil); status != http.StatusOK {
		t.Fatalf("revoke failed with %d", status)
	}

	// The agent must give up rather than retry a token that will never work.
	select {
	case err := <-runErr:
		if !errors.Is(err, relayclient.ErrUnlinked) {
			t.Fatalf("agent stopped with %v, want ErrUnlinked", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the agent kept retrying a revoked token instead of stopping")
	}

	// And the settings must be cleared, which is what sends the next start back
	// to pairing instead of into the same dead end.
	cfg, err := agentcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DeviceToken, cfg.DeviceID = "", ""
	if err := agentcfg.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := agentcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Paired() {
		t.Fatal("after clearing, the agent still thinks it is paired")
	}
	if reloaded.RelayURL == "" {
		t.Error("the relay address was thrown away too; re-pairing would need it typed again")
	}
}
