package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"queueup/internal/store"
)

// Starting an account with nothing but an email address.
//
// The landing page asks for one thing and the next screen is the price. A
// password is asked for after they have paid, which is the first moment
// there is anything in the account worth protecting.
//
// Signing in always needs a password. The one thing an email alone can do is
// get somebody back to the price they were looking at — and only into an
// account that has never had a password and never paid, which is to say an
// account with nothing in it. That matters more than it sounds: TikTok opens
// links in its own browser with its own cookies, so somebody who signed up
// there and taps the reminder email lands in Safari signed out, with no
// password to sign in with. Typing their email again is how they get back.
//
// The moment there is a password or a payment, an email alone gets nobody
// anywhere. Typing the address of somebody who has paid sends a link to that
// person's own inbox, which is the only thing that proves the account is
// theirs.

func (s *Server) startRoutes() {
	s.mux.HandleFunc("POST /api/auth/start", s.handleStart)
	s.mux.HandleFunc("POST /api/auth/first-password", s.withAccount(s.handleFirstPassword))
	s.mux.HandleFunc("POST /api/auth/continue", s.handleContinue)
}

// handleContinue is the link in a reminder email: "pick up where you left
// off", one tap, straight back to the price.
//
// It does exactly what typing the address on the landing page does, by
// going through the very same rules: for an account that has never paid and
// never had a password, back to the paywall; for anything more, a password
// or a link to their own inbox. The token only stands in for typing the
// address, which is why it can be no stronger than that. Anybody opening the
// email has the address in front of them anyway.
//
// It exists because the emails are opened wherever the mail app opens links,
// which is usually not the browser they signed up in. Signed out, the
// paywall could not tell whose offer it was looking at and showed the full
// price — on the one link whose whole job was to show the discounted one.
func (s *Server) handleContinue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account string `json:"a"`
		Token   string `json:"t"`
		Code    string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	from := "ip:" + s.clientIP(r)
	if s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Wait a few minutes and try again.")
		return
	}
	acct, err := s.st.AccountByContinueToken(body.Account, body.Token)
	if err != nil {
		s.signIns.fail(from)
		writeError(w, http.StatusBadRequest, "That link is no longer valid.")
		return
	}
	s.startExisting(w, from, acct.Email, strings.ToUpper(strings.TrimSpace(body.Code)))
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	code := strings.ToUpper(strings.TrimSpace(body.Code))

	from := "ip:" + s.clientIP(r)
	if s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Wait a few minutes and try again.")
		return
	}

	// Somebody coming back to an account they already started is not a new
	// account, and must not be counted against the limit on making them.
	if _, err := s.st.AccountByEmail(email); err == nil {
		s.startExisting(w, from, email, code)
		return
	}
	if s.signUps.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many new accounts from this connection. Try again in an hour.")
		return
	}

	acct, err := s.st.StartAccount(email)
	switch {
	case err == nil:
		s.signUps.fail(from) // counts accounts made, not mistakes
		if err := s.st.MarkLeadConverted(acct.Email); err != nil {
			s.log.Error("marking a lead converted", "err", err)
		}
		s.rememberArrivalCode(acct.ID, code)
		s.issueSession(w, acct, http.StatusCreated)
	case errors.Is(err, store.ErrAccountExists):
		// Made by another request in the moment since the check above.
		s.startExisting(w, from, email, code)
	default:
		s.signIns.fail(from)
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// startExisting decides what typing an email that already has an account
// does. It never gets anybody into an account that has a password or a
// payment behind it.
func (s *Server) startExisting(w http.ResponseWriter, from, email, code string) {
	acct, err := s.st.AccountByEmail(email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check that address. Try again.")
		return
	}
	hasPassword, err := s.st.HasPassword(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check that address. Try again.")
		return
	}
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check that address. Try again.")
		return
	}

	switch {
	case hasPassword:
		// A real account. A password is the only way in.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "You already have an account. Sign in with your password.",
			"sign_in": true,
		})

	case sub.Active():
		// Paid, or given free access, and no password yet: they closed the
		// tab before choosing one. There is something here to protect now, so
		// the way back goes through their own inbox.
		//
		// The same throttle as "forgot password", so this cannot be used to
		// fill somebody's inbox.
		who := "reset:" + email
		if !s.resets.blocked(who) && !s.resets.blocked("reset-ip:"+from) && s.mail.Enabled() {
			s.resets.fail(who)
			s.resets.fail("reset-ip:" + from)
			if token, _, err := s.st.StartPasswordReset(email); err == nil {
				s.emailPasswordLink(acct, token, "Finish setting up QueueUp",
					"You're nearly there: your QueueUp account just needs a password.")
			} else {
				s.log.Error("starting a password link for a paid account", "err", err)
			}
		}
		// The same answer whether or not the email went, so the throttle
		// cannot be watched.
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "We've emailed you a link to finish setting up your account.",
		})

	default:
		// Never had a password, never paid: an empty account. Back to the
		// price they were looking at.
		s.rememberArrivalCode(acct.ID, code)
		s.issueSession(w, acct, http.StatusOK)
	}
}

// rememberArrivalCode records the code somebody arrived on. Last click wins,
// so coming back through a different link moves the attribution to it.
func (s *Server) rememberArrivalCode(accountID, code string) {
	if code == "" || len(code) > 64 {
		return
	}
	if err := s.st.RememberSourceCode(accountID, code); err != nil {
		s.log.Error("remembering where an account came from", "err", err)
	}
}

func (s *Server) issueSession(w http.ResponseWriter, acct store.Account, status int) {
	token, err := s.st.NewSession(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't sign you in. Try again.")
		return
	}
	writeJSON(w, status, map[string]any{
		"session_token": token,
		"account":       map[string]any{"id": acct.ID, "email": acct.Email},
		"created":       status == http.StatusCreated,
	})
}

// handleFirstPassword is the step after paying: choosing the password that
// signing in will need from now on. It only ever sets the first one —
// changing a password needs the current one, in Settings.
func (s *Server) handleFirstPassword(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	err := s.st.SetFirstPassword(acct.ID, body.Password)
	switch {
	case err == nil:
		s.log.Info("first password set", "account", acct.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "password set"})
	case errors.Is(err, store.ErrPasswordAlreadySet):
		writeError(w, http.StatusConflict, "Your account already has a password. Change it in Settings.")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Couldn't find your account.")
	default:
		// The password rule, which is worth saying as it is.
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
