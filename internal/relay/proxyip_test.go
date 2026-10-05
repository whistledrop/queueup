package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"queueup/internal/servers"
	"queueup/internal/store"
)

const testProxyKey = "the-websites-shared-key"

func proxyRig(t *testing.T, key string) *httptest.Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(New(Config{Store: st, Log: quiet, Servers: servers.NewStub(), ProxyKey: key}))
	t.Cleanup(ts.Close)
	return ts
}

// signup is one person typing an email on the landing page, as the website's
// server passes it on. Every call comes from the same place — the test, like
// Netlify — whatever visitor it claims to be for.
func signup(t *testing.T, ts *httptest.Server, email, key, visitor string) int {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/start", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(proxyKeyHeader, key)
	}
	if visitor != "" {
		req.Header.Set(visitorIPHeader, visitor)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// The bug: every visitor arrived through the same few Netlify addresses, so
// five signups in an hour from anywhere closed signup for everybody. With
// the website naming each visitor, twenty different people all get in.
func TestDifferentVisitorsThroughTheWebsiteAreCountedSeparately(t *testing.T) {
	ts := proxyRig(t, testProxyKey)
	for i := 1; i <= 20; i++ {
		visitor := fmt.Sprintf("81.2.69.%d", i)
		if code := signup(t, ts, fmt.Sprintf("person%d@example.com", i), testProxyKey, visitor); code != http.StatusCreated {
			t.Fatalf("visitor %d of 20 was refused (%d): the limit is still counting the website, not them", i, code)
		}
	}
}

// One visitor making account after account still meets the limit.
func TestOneVisitorStillMeetsTheLimit(t *testing.T) {
	ts := proxyRig(t, testProxyKey)
	for i := 1; i <= signUpLimit; i++ {
		if code := signup(t, ts, fmt.Sprintf("same%d@example.com", i), testProxyKey, "81.2.69.7"); code != http.StatusCreated {
			t.Fatalf("signup %d = %d", i, code)
		}
	}
	if code := signup(t, ts, "one-too-many@example.com", testProxyKey, "81.2.69.7"); code != http.StatusTooManyRequests {
		t.Errorf("signup %d from one visitor = %d, want 429", signUpLimit+1, code)
	}
}

// The reason for the key. Without it, naming a fresh address every request
// would make every limit count nothing — so a claim without the key, or with
// the wrong one, is ignored and counted against wherever it really came from.
func TestNamingYourOwnAddressWithoutTheKeyGetsYouNowhere(t *testing.T) {
	for _, c := range []struct {
		name, configured, sent string
	}{
		{"no key sent", testProxyKey, ""},
		{"the wrong key", testProxyKey, "a guess"},
		{"no key configured on the relay at all", "", "anything"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ts := proxyRig(t, c.configured)
			for i := 1; i <= signUpLimit; i++ {
				// A different invented address every time.
				signup(t, ts, fmt.Sprintf("spoof%d@example.com", i), c.sent, fmt.Sprintf("10.0.0.%d", i))
			}
			if code := signup(t, ts, "spoof-last@example.com", c.sent, "10.0.0.99"); code != http.StatusTooManyRequests {
				t.Errorf("an invented address dodged the limit: %d, want 429", code)
			}
		})
	}
}

// Addresses as Netlify sends them, and things that are not addresses at all.
func TestVisitorAddressesAreCleanedOrRefused(t *testing.T) {
	cases := map[string]string{
		"81.2.69.160":            "81.2.69.160",
		" 81.2.69.160 ":          "81.2.69.160",
		"::ffff:81.2.69.160":     "81.2.69.160", // one visitor, not two
		"2a00:23c6:1234:5678::1": "2a00:23c6:1234:5678::1",
		"":                       "",
		"not an address":         "",
		"81.2.69.160, 10.0.0.1":  "", // a list is not an address
		"ip:81.2.69.160":         "",
		strings.Repeat("9", 500): "",
	}
	for in, want := range cases {
		if got := normaliseIP(in); got != want {
			t.Errorf("normaliseIP(%q) = %q, want %q", in, got, want)
		}
	}
}

// Turning somebody away is said out loud, once per window, and never with
// their email address in it.
func TestALimitSaysWhenItTurnsSomebodyAway(t *testing.T) {
	now := time.Now()
	th := newThrottle(2, time.Hour, func() time.Time { return now })
	var said []string
	th.name = "sign-in"
	th.report = func(name, key string) { said = append(said, name+" "+key) }

	for _, key := range []string{"ip:81.2.69.1", "somebody@example.com"} {
		th.fail(key)
		th.fail(key)
		for i := 0; i < 5; i++ {
			th.blocked(key)
		}
	}
	if len(said) != 2 {
		t.Fatalf("reported %d times, want once per key: %v", len(said), said)
	}
	if said[0] != "sign-in ip:81.2.69.1" {
		t.Errorf("first report = %q", said[0])
	}
	if strings.Contains(said[1], "somebody@example.com") {
		t.Errorf("an email address went into the log: %q", said[1])
	}

	// A new window is a new report.
	now = now.Add(2 * time.Hour)
	th.fail("ip:81.2.69.1")
	th.fail("ip:81.2.69.1")
	th.blocked("ip:81.2.69.1")
	if len(said) != 3 {
		t.Errorf("not reported again in a new window: %v", said)
	}
}
