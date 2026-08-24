package servers

import (
	"encoding/json"
	"testing"
)

// A real record from Steam's server list, kept verbatim. Note that "addr"
// carries the QUERY port and "gameport" is the one players connect to: getting
// those the wrong way round means the game is told to connect to a port that
// does not accept players, and status queries time out. This fixture exists so
// that mistake cannot come back.
const realSteamRecord = `{
  "addr": "176.118.33.2:28010",
  "gameport": 28015,
  "steamid": "90291492796101634",
  "name": "Rustopia.gg - EU Main",
  "appid": 252490,
  "gamedir": "rust",
  "players": 54,
  "max_players": 200,
  "map": "Rustopia Mapping",
  "gametype": "mp200,cp54,ptrak,qp312,$r?,v2632,^m,^v,EU,^t,born1786038933,gmrust"
}`

func TestSteamRecordSeparatesGameAndQueryPorts(t *testing.T) {
	var raw steamServer
	if err := json.Unmarshal([]byte(realSteamRecord), &raw); err != nil {
		t.Fatal(err)
	}
	sv := raw.toServer()

	// What Rust connects to.
	if sv.Address != "176.118.33.2:28015" {
		t.Errorf("Address = %q, want the GAME port 28015", sv.Address)
	}
	// What status queries go to.
	if sv.QueryAddress != "176.118.33.2:28010" {
		t.Errorf("QueryAddress = %q, want the query port 28010", sv.QueryAddress)
	}
	if sv.PollAddress() != "176.118.33.2:28010" {
		t.Errorf("PollAddress() = %q, want the query address", sv.PollAddress())
	}
	if sv.Address == sv.QueryAddress {
		t.Error("the two addresses came out identical, so one of them is wrong")
	}

	// The queue length arrives free in the keywords, no extra request needed.
	if sv.Queue != 312 {
		t.Errorf("Queue = %d, want 312 from the qp tag", sv.Queue)
	}
	if sv.Players != 54 || sv.MaxPlayers != 200 {
		t.Errorf("players = %d/%d, want 54/200", sv.Players, sv.MaxPlayers)
	}
	if sv.Name != "Rustopia.gg - EU Main" || !sv.Online {
		t.Errorf("server = %+v", sv)
	}
}

// A server that reports no game port at all must still be usable rather than
// producing an address with a missing port.
func TestSteamRecordWithoutAGamePortFallsBack(t *testing.T) {
	var raw steamServer
	if err := json.Unmarshal([]byte(`{"addr":"10.0.0.1:28015","name":"x"}`), &raw); err != nil {
		t.Fatal(err)
	}
	sv := raw.toServer()
	if sv.Address != "10.0.0.1:28015" {
		t.Errorf("Address = %q, want the reported address", sv.Address)
	}
	if sv.PollAddress() != "10.0.0.1:28015" {
		t.Errorf("PollAddress() = %q", sv.PollAddress())
	}
}

// PollAddress falls back to the connect address when no query address is known,
// which is what the stub and any simpler source will give us.
func TestPollAddressFallsBack(t *testing.T) {
	sv := Server{Address: "1.2.3.4:28015"}
	if got := sv.PollAddress(); got != "1.2.3.4:28015" {
		t.Errorf("PollAddress() = %q, want the address itself", got)
	}
}
