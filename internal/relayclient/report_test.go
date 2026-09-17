package relayclient

import "testing"

// The saved relay address can be written either way round; the report always
// goes to the HTTPS endpoint beside it.
func TestTheReportURLIsDerivedFromTheSavedRelayAddress(t *testing.T) {
	cases := map[string]string{
		"https://queueup-relay.fly.dev":  "https://queueup-relay.fly.dev/agent/report",
		"https://queueup-relay.fly.dev/": "https://queueup-relay.fly.dev/agent/report",
		"wss://queueup-relay.fly.dev":    "https://queueup-relay.fly.dev/agent/report",
		"http://localhost:8080":          "http://localhost:8080/agent/report",
	}
	for in, want := range cases {
		got, err := reportURL(in)
		if err != nil || got != want {
			t.Errorf("reportURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "queueup-relay.fly.dev", "ftp://x"} {
		if _, err := reportURL(bad); err == nil {
			t.Errorf("reportURL(%q) accepted a non-address", bad)
		}
	}
}
