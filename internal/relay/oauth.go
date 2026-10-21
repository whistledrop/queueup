package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"queueup/internal/oidc"
	"queueup/internal/store"
)

// "Continue with Google" and "Continue with Apple", on the website and in the
// app.
//
// The device does the sign-in with Google or Apple and hands the relay the ID
// token it got back. The relay checks that token (package oidc) and then
// finds the account, in this order:
//
//  1. This Google or Apple person is already linked to an account: that one.
//  2. The provider vouches for the email, and an account has that email: link
//     them and sign in. Google and Apple only vouch for an address the person
//     has proved they own, which is the same proof a password reset relies on.
//  3. Otherwise a new account, with the email the provider gave.
//
// Apple's "Hide my email" gives a relay address unique to QueueUp, so it never
// matches an account made on the website with a real address; that person
// gets a new account, which is why the website offers the same buttons.

func (s *Server) oauthRoutes() {
	s.mux.HandleFunc("POST /api/auth/oauth", s.handleOAuth)
}

func (s *Server) provider(name string) *oidc.Provider {
	switch name {
	case "google":
		return s.cfg.Google
	case "apple":
		return s.cfg.Apple
	}
	return nil
}

func (s *Server) handleOAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
		Nonce    string `json:"nonce"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request.")
		return
	}
	from := "ip:" + s.clientIP(r)
	if s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Wait a few minutes and try again.")
		return
	}
	p := s.provider(body.Provider)
	if !p.Enabled() {
		writeError(w, http.StatusNotImplemented, "That way of signing in isn't switched on yet. Use your email instead.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	id, err := p.Verify(ctx, body.IDToken, body.Nonce, time.Now())
	if err != nil {
		s.signIns.fail(from)
		if !errors.Is(err, oidc.ErrInvalid) {
			s.log.Error("checking a sign-in token", "provider", p.Name, "err", err)
		}
		writeError(w, http.StatusUnauthorized, "That sign-in didn't work. Try again.")
		return
	}

	// 1. Already linked.
	if acct, err := s.st.AccountByIdentity(id.Provider, id.Subject); err == nil {
		s.issueSession(w, acct, http.StatusOK)
		return
	}

	// 2. An account with the address the provider vouches for.
	if id.Email != "" && id.EmailVerified {
		if acct, err := s.st.AccountByEmail(id.Email); err == nil {
			if err := s.st.LinkIdentity(acct.ID, id.Provider, id.Subject, id.Email); err != nil {
				s.log.Error("linking a sign-in identity", "err", err)
				writeError(w, http.StatusInternalServerError, "Couldn't sign you in. Try again.")
				return
			}
			s.log.Info("sign-in identity linked", "account", acct.ID, "provider", id.Provider)
			s.issueSession(w, acct, http.StatusOK)
			return
		}
	}

	// 3. Somebody new.
	if id.Email == "" || !strings.Contains(id.Email, "@") {
		// Apple only shares the address the first time somebody signs in to
		// an app. Getting here means they did, and it was lost; the honest
		// way out is email.
		writeError(w, http.StatusBadRequest, "We didn't get an email address from that sign-in. Use your email instead.")
		return
	}
	if s.signUps.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many new accounts from this connection. Try again in an hour.")
		return
	}
	acct, err := s.st.StartAccount(id.Email)
	if errors.Is(err, store.ErrAccountExists) {
		// Only reachable with an address the provider did not vouch for,
		// which must never open somebody else's account.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "There's already an account with that email. Sign in with your password, then link this from Settings.",
			"sign_in": true,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, capitalise(err.Error())+".")
		return
	}
	s.signUps.fail(from)
	if err := s.st.LinkIdentity(acct.ID, id.Provider, id.Subject, id.Email); err != nil {
		s.log.Error("linking a new account's identity", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't finish making your account. Try again.")
		return
	}
	if err := s.st.MarkLeadConverted(acct.Email); err != nil {
		s.log.Error("marking a lead converted", "err", err)
	}
	s.rememberArrivalCode(acct.ID, strings.ToUpper(strings.TrimSpace(body.Code)))
	s.log.Info("account created", "account", acct.ID, "via", id.Provider)
	s.issueSession(w, acct, http.StatusCreated)
}
