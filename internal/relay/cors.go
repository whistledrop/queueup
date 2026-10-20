package relay

import (
	"net/http"
	"net/url"
)

// Cross-origin requests, for local development only.
//
// The website never needs this: it calls the relay from its own server and
// passes the answer on, so no browser is ever making a cross-origin request to
// us. The mobile app does not need it either, because a native app is not a
// browser and the rule does not apply to it.
//
// What needs it is running the mobile app in a browser during development
// (`expo start --web`), which is the only way to look at the app's screens
// without a phone that can run it. Without these headers the browser refuses
// every call and the app looks broken when it is not.
//
// Deliberately narrow:
//   - Only localhost and 127.0.0.1 are allowed, on any port. A real origin on
//     the internet gets nothing, so this cannot be used from a page somebody
//     else controls.
//   - No Access-Control-Allow-Credentials. Authentication here is a Bearer
//     token that a page has to deliberately attach; allowing credentials would
//     let a browser send cookies automatically, which is how cross-site
//     request forgery works. There are no cookies to send, and this keeps it
//     that way.
func allowedDevOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// withDevCORS answers browser preflights and tags responses for the dev
// origins above. Anything else passes through untouched, exactly as before.
func withDevCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedDevOrigin(origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			// The answer depends on the Origin header, so caches must not
			// hand one origin's response to another.
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		// A preflight asks permission and carries no body. It is answered here
		// and never reaches a handler, so this cannot become a way to call
		// something without the browser's checks having passed.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !allowedDevOrigin(origin) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
