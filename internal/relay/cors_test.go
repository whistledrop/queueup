package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The whole point of the rule: a page on the internet must not be able to
// call the relay from somebody's browser. Only a developer's own machine can.
func TestOnlyLocalhostIsAllowedCrossOrigin(t *testing.T) {
	cases := []struct {
		origin string
		allow  bool
	}{
		{"http://localhost:8099", true},
		{"http://localhost:19006", true},
		{"http://127.0.0.1:8081", true},
		{"https://localhost:3000", true},
		// Everything else, including our own website: it calls from its own
		// server and never needs this.
		{"https://queueuprust.com", false},
		{"https://evil.example", false},
		// The lookalikes a sloppy check would let through.
		{"https://localhost.evil.example", false},
		{"https://notlocalhost", false},
		{"http://127.0.0.1.evil.example", false},
		{"", false},
	}

	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			h := withDevCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/api/billing", nil)
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
			if c.allow && got != c.origin {
				t.Errorf("origin %q was not allowed (header %q)", c.origin, got)
			}
			if !c.allow && got != "" {
				t.Errorf("origin %q was allowed, and must not be (header %q)", c.origin, got)
			}
			// Credentials are never allowed, for any origin: there are no
			// cookies here and allowing them would invite the one attack this
			// header exists to prevent.
			if cred := rec.Header().Get("Access-Control-Allow-Credentials"); cred != "" {
				t.Errorf("credentials allowed for %q: %q", c.origin, cred)
			}
		})
	}
}

// A preflight from an allowed origin is answered without reaching a handler;
// one from anywhere else is refused outright.
func TestPreflight(t *testing.T) {
	reached := false
	h := withDevCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/jobs", nil)
	req.Header.Set("Origin", "http://localhost:8099")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("allowed preflight = %d, want 204", rec.Code)
	}
	if reached {
		t.Error("a preflight reached a handler; it carries no body and must not")
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/jobs", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("refused preflight = %d, want 403", rec.Code)
	}
}

// Ordinary requests with no Origin at all, which is every agent and every
// call the website makes, must be completely unaffected.
func TestRequestsWithoutAnOriginAreUntouched(t *testing.T) {
	h := withDevCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/billing", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Errorf("handler not reached: %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("a request with no Origin got CORS headers")
	}
}
