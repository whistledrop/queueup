package relay

import (
	"encoding/json"
	"errors"
	"net/http"

	"queueup/internal/store"
)

// The web app signs in with an email and a password, and gets back a session
// token that its own server keeps in an http-only cookie. The browser never sees
// the token, so nothing running on the page can leak it.
//
// The brief allowed either magic links or a plain password. Password won for one
// practical reason: magic links need an email provider and there is not one yet.
// Phase 4 introduces email for notifications, and a magic link can be added
// alongside these routes without changing any of them.

func (s *Server) authRoutes() {
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", s.withAccount(s.handleMe))
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	acct, err := s.st.Register(c.Email, c.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
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
	acct, token, err := s.st.SignIn(c.Email, c.Password)
	if err != nil {
		if errors.Is(err, store.ErrBadCredentials) {
			writeError(w, http.StatusUnauthorized, "That email or password isn't right.")
			return
		}
		s.log.Error("signing in", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't sign you in. Try again in a moment.")
		return
	}
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
	writeJSON(w, http.StatusOK, map[string]any{
		"id": acct.ID, "email": acct.Email, "created_at": acct.CreatedAt,
	})
}
