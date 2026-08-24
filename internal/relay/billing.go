package relay

import (
	"net/http"

	"queueup/internal/store"
)

// Billing: one gate, in one place, checked on the server.
//
// The rule Logan chose: setting up is free (account, pairing, seeing the PC
// online), and the gate sits on joining. Nobody pays until their own setup has
// visibly worked, and nobody joins without paying. The gate is enforced HERE,
// on the relay, not in the browser: the web app's checks are courtesy, this is
// the law.
//
// Until Stripe is connected, billing is switched off entirely
// (QUEUEUP_BILLING unset) and everything behaves exactly as before: free.
// Turning it on later is an environment variable, not a deploy.

// Price mirrors web/lib/pricing.ts. When Stripe exists, its Price object is
// the master and both of these follow it.
const (
	priceMonthlyPence = 499
	priceCurrency     = "GBP"
	priceLine         = "£4.99 a month"
)

func (s *Server) billingRoutes() {
	s.mux.HandleFunc("GET /api/billing", s.withAccount(s.handleBilling))
	s.mux.HandleFunc("POST /api/billing/checkout", s.withAccount(s.handleCheckout))
	// When Stripe arrives, its webhook lands here: checkout.session.completed
	// sets the subscription active, customer.subscription.deleted sets it back
	// to none. Signature checked with the webhook secret, then st.SetSubscription.
	// s.mux.HandleFunc("POST /billing/webhook", s.handleStripeWebhook)
}

// requireSubscription is the gate. It returns true when the request may
// proceed. When it returns false it has already written the refusal, with a
// message written for a person and status 402 so the web app can tell "needs
// to pay" apart from every other failure.
func (s *Server) requireSubscription(w http.ResponseWriter, acct store.Account) bool {
	if !s.cfg.BillingEnabled {
		return true
	}
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		s.log.Error("reading subscription", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription. Try again in a moment.")
		return false
	}
	if sub.Active() {
		return true
	}
	writeError(w, http.StatusPaymentRequired,
		"Joining needs the QueueUp subscription, "+priceLine+". Setting up is free; this is the only paid part.")
	return false
}

func (s *Server) handleBilling(w http.ResponseWriter, r *http.Request, acct store.Account) {
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       s.cfg.BillingEnabled,
		"subscribed":    !s.cfg.BillingEnabled || sub.Active(),
		"price_pence":   priceMonthlyPence,
		"price_line":    priceLine,
		"currency":      priceCurrency,
		"subscribed_at": sub.SubscribedAt,
	})
}

// handleCheckout will create a Stripe Checkout session and return its URL for
// the browser to go to. Until Stripe is connected it says so honestly.
func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if !s.cfg.BillingEnabled {
		writeError(w, http.StatusNotImplemented,
			"Payments aren't switched on yet, so your account runs free. Just tap Join.")
		return
	}
	// TODO(stripe): create a Checkout session (mode=subscription, the £4.99
	// price, client_reference_id=acct.ID, success/cancel URLs from the web app)
	// and return {"url": session.URL}. The webhook above completes the loop.
	writeError(w, http.StatusNotImplemented,
		"Checkout isn't wired up yet. This is where the Stripe page will open.")
	_ = acct
}
