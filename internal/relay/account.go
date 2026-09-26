package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
	s.mux.HandleFunc("POST /api/account/erase/cancel", s.withAccount(s.handleKeepAccount))
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

	// Nothing is deleted today. People ask for this on bad nights, and an
	// instant, silent, irreversible delete is how somebody loses six months of
	// wipe-day schedules over an argument. The week costs us nothing and is
	// the only chance they get to be wrong.
	when, err := s.st.RequestErasure(acct.ID)
	if err != nil {
		s.log.Error("scheduling an erasure", "account", acct.ID, "err", err)
		writeError(w, http.StatusInternalServerError,
			"Couldn't do that. Nothing was changed. Tell us on the feedback page and we'll do it by hand.")
		return
	}
	s.log.Info("account deletion requested", "account", acct.ID, "erase_after", when)

	// And they are told, by email, because the account this protects most is
	// the one somebody else got into: if a stranger deletes it, the owner has
	// a week and a message telling them how to stop it.
	s.tellThemTheyAreLeaving(r.Context(), acct, when)

	writeJSON(w, http.StatusOK, map[string]any{
		"erase_after": when,
		"status": "Your account will be deleted on " + when.Format("Monday 2 January") +
			". Sign in before then if you change your mind.",
	})
}

// handleKeepAccount is the change of mind.
func (s *Server) handleKeepAccount(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if err := s.st.CancelErasure(acct.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Already cancelled, or never asked. Either way they have what they
			// want, so this is not an error to shout about.
			writeJSON(w, http.StatusOK, map[string]string{"status": "Your account is staying."})
			return
		}
		s.log.Error("cancelling an erasure", "account", acct.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't do that. Try again in a moment.")
		return
	}
	s.log.Info("account deletion cancelled", "account", acct.ID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "Your account is staying. Nothing has been deleted.",
	})
}

func (s *Server) tellThemTheyAreLeaving(ctx context.Context, acct store.Account, when time.Time) {
	if !s.mail.Enabled() {
		return
	}
	body := "Somebody asked to delete your QueueUp account.\n\n" +
		"Nothing has been deleted yet. On " + when.Format("Monday 2 January") +
		" your account and everything\nin it will be erased: your linked PC, your joins, your schedules and your\n" +
		"saved servers. That cannot be undone.\n\n" +
		"If you meant to do this, you do not need to do anything.\n\n" +
		"If this was not you, sign in at " + strings.TrimSuffix(s.cfg.WebURL, "/") + "/settings" + " and press \"Keep my account\"\n" +
		"on the Settings page. Change your password while you are there.\n"
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.mail.Send(sendCtx, acct.Email, "Your QueueUp account is scheduled for deletion", body); err != nil {
		s.log.Error("sending a deletion notice", "account", acct.ID, "err", err)
	}
}

// eraseSweep is how the week actually ends: accounts past their day are
// erased, once, by the relay itself.
func (s *Server) RunErasureSweep(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Hour
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.eraseSweep(time.Now())
		}
	}
}

func (s *Server) eraseSweep(now time.Time) {
	due, err := s.st.AccountsDueForErasure(now)
	if err != nil {
		s.log.Error("finding accounts due for erasure", "err", err)
		return
	}
	for _, a := range due {
		// The PC is told to stop before its rows go, so it does not carry on
		// running a join for an account that no longer exists.
		if devices, err := s.st.Devices(a.ID); err == nil {
			for _, d := range devices {
				s.hub.Disconnect(d.ID)
			}
		}
		if err := s.st.EraseAccount(a.ID); err != nil {
			s.log.Error("erasing an account whose week is up", "account", a.ID, "err", err)
			continue
		}
		s.log.Info("account erased after its week", "account", a.ID)
	}
}
