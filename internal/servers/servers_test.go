package servers

import (
	"context"
	"strings"
	"testing"
)

func TestStubSearchMatchesOnName(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	all, err := s.Search(ctx, "", 100)
	if err != nil || len(all) < 3 {
		t.Fatalf("empty search returned %d servers, %v", len(all), err)
	}

	moose, err := s.Search(ctx, "moose", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(moose) != 1 || !strings.Contains(strings.ToLower(moose[0].Name), "moose") {
		t.Fatalf("search for moose returned %v", moose)
	}

	none, err := s.Search(ctx, "definitely-not-a-server", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("nonsense search returned %d servers, want 0", len(none))
	}
}

func TestStubByID(t *testing.T) {
	s := NewStub()
	sv, err := s.ByID(context.Background(), "stub-1")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Address == "" {
		t.Fatal("a server came back with no address, so nothing could connect to it")
	}
	if _, err := s.ByID(context.Background(), "nope"); err == nil {
		t.Fatal("an unknown id was accepted")
	}
}

// The offline case matters: a server that is down must not claim to have
// players on it, or the wipe-restart logic would never trigger.
func TestStubOfflineServerReportsNoPlayers(t *testing.T) {
	s := NewStub()
	all, _ := s.Search(context.Background(), "", 100)
	found := false
	for _, sv := range all {
		if sv.Online {
			continue
		}
		found = true
		if sv.Players != 0 || sv.Queue != 0 {
			t.Errorf("offline server %q reports %d players and %d queued",
				sv.Name, sv.Players, sv.Queue)
		}
	}
	if !found {
		t.Error("the built-in list has no offline server, so that case is never exercised")
	}
}

func TestFromEnvExplainsWhatIsMissing(t *testing.T) {
	t.Setenv("QUEUEUP_SERVER_SOURCE", "steam")
	t.Setenv("QUEUEUP_STEAM_API_KEY", "")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("a missing Steam key was accepted")
	}
	// The message has to say which variable to set and where to get the value,
	// because the person reading it is not a developer.
	msg := err.Error()
	for _, want := range []string{"QUEUEUP_STEAM_API_KEY", "steamcommunity.com"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}

	t.Setenv("QUEUEUP_SERVER_SOURCE", "battlemetrics")
	t.Setenv("QUEUEUP_BATTLEMETRICS_TOKEN", "")
	_, err = FromEnv()
	if err == nil {
		t.Fatal("a missing BattleMetrics token was accepted")
	}
	if !strings.Contains(err.Error(), "paid subscription") {
		t.Errorf("the BattleMetrics error does not warn that it costs money: %q", err)
	}
}

func TestFromEnvDefaultsToTheBuiltInList(t *testing.T) {
	t.Setenv("QUEUEUP_SERVER_SOURCE", "")
	p, err := FromEnv()
	if err != nil || p.Name() != "stub" {
		t.Fatalf("FromEnv with nothing set = %v, %v; want the stub", p, err)
	}
}
