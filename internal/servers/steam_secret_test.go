package servers

import (
	"strings"
	"testing"
)

// The Steam key travels in the query string, so Go's own network errors quote it
// back with the whole URL in them. Logging one of those puts the key in the
// relay's logs permanently, where it is readable by anyone who can read logs and
// long outlives the request that failed.
func TestSteamKeyNeverSurvivesIntoAnErrorMessage(t *testing.T) {
	const key = "7B63CFE0724B12D93B6597345D1B9857"
	raw := `Get "https://api.steampowered.com/IGameServersService/GetServerList/v1/?filter=%5Cappid%5C252490&key=` +
		key + `&limit=50": context deadline exceeded`

	got := scrubKey(raw)
	if strings.Contains(got, key) {
		t.Fatalf("the API key survived into the message: %q", got)
	}
	// It still has to be a useful error afterwards.
	if !strings.Contains(got, "context deadline exceeded") {
		t.Errorf("scrubbing threw away the actual problem: %q", got)
	}
	if !strings.Contains(got, "api.steampowered.com") {
		t.Errorf("scrubbing threw away where it was going: %q", got)
	}
}

func TestScrubLeavesOrdinaryTextAlone(t *testing.T) {
	const msg = "couldn't reach Steam's server list: no such host"
	if got := scrubKey(msg); got != msg {
		t.Errorf("scrubKey changed a message with no key in it: %q", got)
	}
}
