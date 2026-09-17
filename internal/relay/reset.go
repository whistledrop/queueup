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

	link := fmt.Sprintf("%s/reset?token=%s", strings.TrimSuffix(s.cfg.WebURL, "/"), token)
	text := "Somebody asked to reset the password for your QueueUp account.\n\n" +
		"Open this link to choose a new one. It works for one hour, once:\n\n" +
		link + "\n\n" +
		"If that was not you, you can ignore this email. Your password has not changed,\n" +
		"and nobody can use this link without opening it from your inbox.\n\n" +
		"QueueUp is a free beta. It never asks for your Steam password.\n"

	// Sent in the background: the person is staring at a page, and how long
	// Resend takes is not their business.
	go func(to string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, to, "Reset your QueueUp password", text); err != nil {
			s.log.Error("sending a password reset email", "err", err)
			return
		}
		s.log.Info("password reset email sent", "account", acct.ID)
	}(acct.Email)

	answer()
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
