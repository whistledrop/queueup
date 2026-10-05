package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
)

// Forgotten passwords.
//
// Two rules shape this whole file. The website's answer never reveals whether
// an address has an account, because a page that does is a way of harvesting
// customers. And a reset always ends every session, because the reason
// somebody resets may be that another person is already signed in as them.

// resetRequestLimit is how many reset emails one address, or one connection,
// can ask for in resetRequestWindow. Enough for somebody fumbling; not enough
// to use QueueUp to post mail at anybody.
const (
	resetRequestLimit  = 4
	resetRequestWindow = time.Hour
)

func (s *Server) resetRoutes() {
	s.mux.HandleFunc("POST /api/auth/forgot", s.handleForgotPassword)
	s.mux.HandleFunc("POST /api/auth/reset", s.handleResetPassword)
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that. Try again.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))

	// The same answer whatever happens next, including when the address is
	// unknown or the email fails to send. Anything else leaks.
	const same = "If that address has a QueueUp account, a reset link is on its way. It works for one hour."
	answer := func() {
		writeJSON(w, http.StatusOK, map[string]string{"status": same})
	}

	who, from := "reset:"+email, "reset-ip:"+clientIP(r)
	if s.resets.blocked(who) || s.resets.blocked(from) {
		// Even the throttle answers the same way, so it cannot be used to
		// discover which addresses exist.
		answer()
		return
	}
	if email == "" {
		answer()
		return
	}
	s.resets.fail(who)
	s.resets.fail(from)

	if !s.mail.Enabled() {
		s.log.Error("a password reset was asked for but email is not set up on this relay")
		answer()
		return
	}

	token, acct, err := s.st.StartPasswordReset(email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("starting a password reset", "err", err)
		}
		answer()
		return
	}

	s.emailPasswordLink(acct, token, "Reset your QueueUp password",
		"Somebody asked to reset the password for your QueueUp account.")

	answer()
}

// emailPasswordLink sends somebody a one-hour, one-use link to choose a
// password, in the background: the person is staring at a page, and how long
// Resend takes is not their business.
//
// Two people need this. Somebody who forgot their password, and somebody who
// paid without ever setting one — they closed the tab before that step, and
// signing in needs a password, so a link to their inbox is the only way back
// that proves the account is theirs.
func (s *Server) emailPasswordLink(acct store.Account, token, subject, opening string) {
	link := fmt.Sprintf("%s/reset?token=%s", strings.TrimSuffix(s.cfg.WebURL, "/"), token)
	text := opening + "\n\n" +
		"Open this link to choose your password. It works for one hour, once:\n\n" +
		link + "\n\n" +
		"If that was not you, you can ignore this email. Nothing has changed,\n" +
		"and nobody can use this link without opening it from your inbox.\n\n" +
		"QueueUp never asks for your Steam password.\n"
	go func(to string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, to, subject, text); err != nil {
			s.log.Error("sending a password link", "err", err)
			return
		}
		s.log.Info("password link sent", "account", acct.ID)
	}(acct.Email)
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that. Try again.")
		return
	}
	err := s.st.FinishPasswordReset(strings.TrimSpace(body.Token), body.Password)
	if errors.Is(err, store.ErrResetInvalid) {
		writeError(w, http.StatusBadRequest,
			"That reset link is no longer valid. Ask for a new one.")
		return
	}
	if err != nil {
		// The only other failure is the password rule, which is worth saying.
		writeError(w, http.StatusBadRequest, capitalise(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "Your password is changed. You can sign in with it now.",
	})
}
