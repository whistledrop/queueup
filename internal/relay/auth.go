package relay

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
)

// clientIP is who is asking, as best we can tell, and the thing every limit
// on guessing and on making accounts is counted against.
//
// Behind Fly the real address arrives in a header; the connection address is
// Fly's own proxy. But most requests do not come from a visitor at all: they
// come from the website's server, calling on a visitor's behalf, so Fly sees
// Netlify. Counted that way, every visitor in the world shares a handful of
// addresses, and five signups in an hour from anywhere shut signup for
// everybody.
//
// So the website passes the visitor's own address along, with the shared
// key that proves the website is the one saying so. Without the key the
// claim is ignored: if any caller could name its own address, it could name
// a fresh one every request and every limit here would count nothing.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.ProxyKey != "" {
		key := r.Header.Get(proxyKeyHeader)
		if key != "" && subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.ProxyKey)) == 1 {
			if ip := normaliseIP(r.Header.Get(visitorIPHeader)); ip != "" {
				return ip
			}
		}
	}
	if v := r.Header.Get("Fly-Client-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// First entry is the original client; the rest are proxies.
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// The two headers the website sends when it calls on a visitor's behalf.
const (
	proxyKeyHeader  = "X-QueueUp-Proxy-Key"
	visitorIPHeader = "X-QueueUp-Visitor-IP"
)

// normaliseIP returns a clean address, or "" for anything that is not one.
//
// An IPv4 address can arrive wrapped as IPv6 ("::ffff:1.2.3.4"); unwrapped, so
// one visitor is never counted as two. Anything that does not parse is
// refused rather than used as a key, so the header cannot carry arbitrary
// text into the limiter.
func normaliseIP(v string) string {
	ip := net.ParseIP(strings.TrimSpace(v))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// The web app signs in with an email and a password, and gets back a session
// token that its own server keeps in an http-only cookie. The browser never sees
// the token, so nothing running on the page can leak it.
//
// The brief allowed either magic links or a plain password. Password won for one
// practical reason: magic links need an email provider and there is not one yet.
// Phase 4 introduces email for notifications, and a magic link can be added
// alongside these routes without changing any of them.

// signInLimit is how many failed sign-ins one email address, or one address on
// the internet, may rack up before it has to wait. Generous enough that nobody
// fumbling their own password ever meets it.
const (
	signInLimit  = 8
	signInWindow = 15 * time.Minute
)

func (s *Server) authRoutes() {
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", s.withAccount(s.handleMe))
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Code is whatever they arrived on, carried through from the link they
	// clicked. Recorded at sign-up rather than only at checkout, because
	// otherwise the channel that brought somebody is only ever known for the
	// ones who paid, and the interesting question is which channel brings
	// people who DON'T.
	Code string `json:"code"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}

	from := "ip:" + s.clientIP(r)
	if s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests,
			"Too many attempts. Wait a few minutes and try again.")
		return
	}
	if s.signUps.blocked(from) {
		writeError(w, http.StatusTooManyRequests,
			"Too many new accounts from this connection. Try again in an hour.")
		return
	}

	acct, err := s.st.Register(c.Email, c.Password)
	if err != nil {
		s.signIns.fail(from)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.signUps.fail(from) // counts accounts made, not mistakes
	// They gave this address on the landing page and have now come back and
	// finished. The lead stops being a lead rather than being counted twice.
	if err := s.st.MarkLeadConverted(acct.Email); err != nil {
		s.log.Error("marking a lead converted", "err", err)
	}
	if c.Code != "" {
		if err := s.st.RememberSourceCode(acct.ID, c.Code); err != nil {
			s.log.Error("remembering where a new account came from", "err", err)
		}
	}
	token, err := s.st.NewSession(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"Account created, but signing you in failed. Try signing in.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_token": token,
		"account":       map[string]any{"id": acct.ID, "email": acct.Email},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}

	// Two keys: the account being aimed at, and where the attempts come from.
	// The first stops one account being ground down, the second stops one
	// source working through a list of accounts.
	who := strings.ToLower(strings.TrimSpace(c.Email))
	from := "ip:" + s.clientIP(r)
	if s.signIns.blocked(who) || s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests,
			"Too many sign-in attempts. Wait a few minutes and try again.")
		return
	}

	acct, token, err := s.st.SignIn(c.Email, c.Password)
	if err != nil {
		if errors.Is(err, store.ErrBadCredentials) {
			s.signIns.fail(who)
			s.signIns.fail(from)
			writeError(w, http.StatusUnauthorized, "That email or password isn't right.")
			return
		}
		s.log.Error("signing in", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't sign you in. Try again in a moment.")
		return
	}
	s.signIns.reset(who)
	s.signIns.reset(from)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_token": token,
		"account":       map[string]any{"id": acct.ID, "email": acct.Email},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := bearer(r); token != "" {
		if err := s.st.SignOut(token); err != nil {
			s.log.Error("signing out", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, acct store.Account) {
	out := map[string]any{"id": acct.ID, "email": acct.Email, "created_at": acct.CreatedAt}
	// Somebody counting down to deletion must be reminded of it everywhere,
	// not only on the page where they asked.
	if when, leaving := acct.LeavingOn(); leaving {
		out["erase_after"] = when
	}
	// Somebody who paid but has not chosen a password yet is sent to choose
	// one before anything else: until they do, they could not sign in again
	// anywhere but this browser.
	if has, err := s.st.HasPassword(acct.ID); err == nil {
		out["has_password"] = has
	}
	writeJSON(w, http.StatusOK, out)
}
