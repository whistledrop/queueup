package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"queueup/internal/store"
)

// The two things somebody must be able to do to their own account without
// asking us: change the password, and leave.
//
// Both were missing. Changing a password meant signing out and using
// "forgotten password", which asks somebody to prove they can read their email
// before letting them pick a better password. And leaving meant emailing us,
// which under UK data protection law is a right we have to honour by hand,
// every time, forever. Neither is acceptable once real customers arrive.

func (s *Server) accountRoutes() {
	s.mux.HandleFunc("POST /api/auth/password", s.withAccount(s.handleChangePassword))
	s.mux.HandleFunc("POST /api/account/erase", s.withAccount(s.handleEraseOwnAccount))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}

	// Guessing the current password here is the same attack as guessing it at
	// the sign-in page, so it meets the same limit.
	from := "ip:" + clientIP(r)
	if s.signIns.blocked(acct.ID) || s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests,
			"Too many attempts. Wait a few minutes and try again.")
		return
	}

	if err := s.st.ChangePassword(acct.ID, body.Current, body.Next); err != nil {
		switch {
		case errors.Is(err, store.ErrBadCredentials):
			s.signIns.fail(acct.ID)
			s.signIns.fail(from)
			writeError(w, http.StatusUnauthorized, "That isn't your current password.")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusUnauthorized, "Sign in again.")
		default:
			// CheckPassword's complaints are written for the person reading
			// them, so they go straight through.
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	s.signIns.reset(acct.ID)

	// Changing the password killed every session, including this browser's.
	// A fresh one goes back so the person who just did it stays where they
	// are, while anything signed in elsewhere is now out.
	token, err := s.st.NewSession(acct.ID)
	if err != nil {
		s.log.Error("starting a session after a password change", "account", acct.ID, "err", err)
		writeError(w, http.StatusInternalServerError,
			"Your password was changed, but you'll need to sign in again.")
		return
	}
	s.log.Info("password changed", "account", acct.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_token": token,
		"status":        "Your password is changed. Anything else signed in has been signed out.",
	})
}

func (s *Server) handleEraseOwnAccount(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}

	// Money first. Erasing the account does not reach into Stripe, so leaving
	// while a subscription is live would delete everything we know about them
	// and keep charging their card, with no account left to cancel from. They
	// cancel, then they leave.
	if s.cfg.BillingEnabled {
		if sub, err := s.st.SubscriptionFor(acct.ID); err == nil && sub.Active() {
			writeError(w, http.StatusConflict,
				"Cancel your subscription first, or you would keep being charged with no account to cancel from. "+
					"Use Manage subscription, cancel there, then come back and delete.")
			return
		}
	}

	from := "ip:" + clientIP(r)
	if s.signIns.blocked(acct.ID) || s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests,
			"Too many attempts. Wait a few minutes and try again.")
		return
	}
	if err := s.st.CheckOwnPassword(acct.ID, body.Password); err != nil {
		s.signIns.fail(acct.ID)
		s.signIns.fail(from)
		writeError(w, http.StatusUnauthorized, "That password isn't right.")
		return
	}
	// Typing the word is the last gate. A password can be saved in a browser;
	// this cannot be clicked through by accident.
	if !strings.EqualFold(strings.TrimSpace(body.Confirm), "DELETE") {
		writeError(w, http.StatusBadRequest, `Type DELETE in the box to confirm.`)
		return
	}

	// Any PC linked to this account is told to stop before its rows go, so it
	// does not carry on running a join for an account that no longer exists.
	if devices, err := s.st.Devices(acct.ID); err == nil {
		for _, d := range devices {
			s.hub.Disconnect(d.ID)
		}
	}
	if err := s.st.EraseAccount(acct.ID); err != nil {
		s.log.Error("erasing an account at its owner's request", "account", acct.ID, "err", err)
		writeError(w, http.StatusInternalServerError,
			"Couldn't delete the account. Nothing was changed. Tell us on the feedback page and we'll do it by hand.")
		return
	}
	s.log.Info("account erased at its owner's request", "account", acct.ID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "Your account and everything in it is gone.",
	})
}
