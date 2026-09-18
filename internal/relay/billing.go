package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
	"queueup/internal/stripe"
)

// Billing: one gate, in one place, checked on the server.
//
// The rule Logan chose: setting up is free (account, pairing, seeing the PC
// online), and the gate sits on joining. Nobody pays until their own setup has
// visibly worked, and nobody joins without paying. The gate is enforced HERE,
// on the relay, not in the browser: the web app's checks are courtesy, this is
// the law.
//
// Two switches, deliberately separate:
//   - Stripe connected (QUEUEUP_STRIPE_SECRET_KEY and friends): checkout and the
//     manage page work, and webhooks keep each account's status in step.
//   - QUEUEUP_BILLING=on: the gate actually closes for people who have not paid.
//
// So the whole payment flow can be tested on the live site, in Stripe's test
// mode, while every beta tester carries on joining for free.
//
// Price: £1.99 for the first month, then £4.99 a month. Stripe holds the real
// numbers; these mirror web/lib/pricing.ts for the words on screen.
const (
	priceMonthlyPence = 499
	priceIntroPence   = 199
	priceCurrency     = "GBP"
	priceLine         = "£4.99 a month"
	introLine         = "£1.99 for your first month, then £4.99 a month"
)

func (s *Server) billingRoutes() {
	s.mux.HandleFunc("GET /api/billing", s.withAccount(s.handleBilling))
	s.mux.HandleFunc("POST /api/billing/checkout", s.withAccount(s.handleCheckout))
	s.mux.HandleFunc("POST /api/billing/portal", s.withAccount(s.handlePortal))
	s.mux.HandleFunc("POST /stripe/webhook", s.handleStripeWebhook)
}

// stripeReady reports whether checkout can actually run.
func (s *Server) stripeReady() bool {
	return s.cfg.Stripe.Enabled() && s.cfg.StripePriceID != ""
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
	offer := priceLine
	if !sub.IntroUsed && s.cfg.StripeIntroCouponID != "" {
		offer = introLine
	}
	writeError(w, http.StatusPaymentRequired,
		"Joining needs the QueueUp subscription: "+offer+". Setting up is free; this is the only paid part.")
	return false
}

func (s *Server) handleBilling(w http.ResponseWriter, r *http.Request, acct store.Account) {
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription.")
		return
	}
	intro := !sub.IntroUsed && s.cfg.StripeIntroCouponID != ""
	line := priceLine
	if intro {
		line = introLine
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         s.cfg.BillingEnabled,
		"subscribed":      !s.cfg.BillingEnabled || sub.Active(),
		"paying":          sub.Active(),
		"price_pence":     priceMonthlyPence,
		"intro_pence":     priceIntroPence,
		"intro_available": intro,
		"price_line":      line,
		"currency":        priceCurrency,
		"subscribed_at":   sub.SubscribedAt,
		"can_manage":      sub.CustomerID != "" && s.cfg.Stripe.Enabled(),
		"checkout_ready":  s.stripeReady(),
		"test_mode":       s.cfg.Stripe.TestMode(),
	})
}

// handleCheckout creates a Stripe checkout page and returns its address for
// the browser to go to.
func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if !s.stripeReady() {
		writeError(w, http.StatusNotImplemented,
			"Payments aren't switched on yet, so your account runs free. Just tap Join.")
		return
	}
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription.")
		return
	}
	// Paying twice for one thing is the complaint that ends in a chargeback.
	if sub.Active() {
		writeError(w, http.StatusConflict,
			"You're already subscribed. Use Manage subscription to change or cancel it.")
		return
	}
	coupon := ""
	if !sub.IntroUsed {
		coupon = s.cfg.StripeIntroCouponID
	}
	web := strings.TrimSuffix(s.cfg.WebURL, "/")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	url, err := s.cfg.Stripe.CreateCheckoutSession(ctx, stripe.CheckoutParams{
		AccountID:  acct.ID,
		Email:      acct.Email,
		CustomerID: sub.CustomerID,
		PriceID:    s.cfg.StripePriceID,
		CouponID:   coupon,
		SuccessURL: web + "/?subscribed=1",
		CancelURL:  web + "/subscribe",
	})
	if err != nil {
		s.log.Error("creating a checkout page", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway, "Couldn't open the payment page. Try again in a moment.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// handlePortal opens Stripe's own page for changing card, seeing receipts and
// cancelling. Cancelling is two taps away on purpose: the fastest way to lose
// somebody's trust is to make leaving hard.
func (s *Server) handlePortal(w http.ResponseWriter, r *http.Request, acct store.Account) {
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription.")
		return
	}
	if !s.cfg.Stripe.Enabled() || sub.CustomerID == "" {
		writeError(w, http.StatusNotFound, "There's no subscription on this account to manage.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	url, err := s.cfg.Stripe.CreatePortalSession(ctx, sub.CustomerID, strings.TrimSuffix(s.cfg.WebURL, "/")+"/")
	if err != nil {
		s.log.Error("opening the manage page", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway, "Couldn't open the subscription page. Try again in a moment.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// handleStripeWebhook keeps each account's paid status in step with Stripe.
//
// Answering matters as much as acting. Anything Stripe gets a 2xx for, it
// forgets; anything else, it retries for days. So: a forged or garbled request
// is refused, a real event about somebody QueueUp does not know is accepted and
// logged (retrying will not make them exist), and only a failure on our side
// that retrying could fix gets a 500.
func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that.")
		return
	}
	ev, err := stripe.VerifyWebhook(payload, r.Header.Get("Stripe-Signature"), s.cfg.StripeWebhookSecret, time.Now())
	if err != nil {
		s.log.Warn("refused a webhook that did not come from Stripe", "err", err)
		writeError(w, http.StatusBadRequest, "Signature check failed.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch ev.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(ev.Data.Object, &cs); err != nil || cs.Mode != "subscription" || cs.Subscription == "" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		if _, err := s.st.AccountByID(cs.ClientReferenceID); err != nil {
			s.log.Error("a checkout finished for an account we do not have", "event", ev.ID)
			writeJSON(w, http.StatusOK, map[string]string{"status": "unknown account"})
			return
		}
		if err := s.st.LinkStripe(cs.ClientReferenceID, cs.Customer, cs.Subscription); err != nil {
			s.log.Error("linking a Stripe customer", "err", err)
			writeError(w, http.StatusInternalServerError, "Try again.")
			return
		}
		if err := s.syncSubscription(ctx, cs.ClientReferenceID, cs.Subscription); err != nil {
			s.log.Error("reading a new subscription from Stripe", "err", err)
			writeError(w, http.StatusInternalServerError, "Try again.")
			return
		}

	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(ev.Data.Object, &sub); err != nil || sub.ID == "" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		accountID := sub.Metadata["account_id"]
		if accountID == "" {
			accountID, _ = s.st.AccountForStripeCustomer(sub.Customer)
		}
		if _, err := s.st.AccountByID(accountID); err != nil {
			s.log.Error("a subscription changed for an account we do not have", "event", ev.ID)
			writeJSON(w, http.StatusOK, map[string]string{"status": "unknown account"})
			return
		}
		if err := s.syncSubscription(ctx, accountID, sub.ID); err != nil {
			s.log.Error("reading a subscription from Stripe", "err", err)
			writeError(w, http.StatusInternalServerError, "Try again.")
			return
		}

	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// syncSubscription copies a subscription's CURRENT state from Stripe onto the
// account, instead of trusting whatever the webhook carried. Webhooks arrive
// late and out of order; "updated" can land after "deleted". Asking Stripe
// what is true now is the only way that cannot go wrong.
func (s *Server) syncSubscription(ctx context.Context, accountID, subID string) error {
	live, err := s.cfg.Stripe.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}
	current, err := s.st.SubscriptionFor(accountID)
	if err != nil {
		return err
	}
	if stripe.GrantsAccess(live.Status) {
		if err := s.st.LinkStripe(accountID, live.Customer, live.ID); err != nil {
			return err
		}
		if !current.Active() || current.SubID != live.ID {
			s.log.Info("subscription active", "account", accountID, "status", live.Status)
			return s.st.SetSubscription(accountID, "active", live.ID)
		}
		return nil
	}
	// An old subscription ending must not switch off a newer one. Only the
	// subscription the account is currently on can close the gate.
	if current.SubID != "" && current.SubID != live.ID {
		return nil
	}
	if current.Active() {
		s.log.Info("subscription ended", "account", accountID, "status", live.Status)
	}
	return s.st.SetSubscription(accountID, "none", live.ID)
}
