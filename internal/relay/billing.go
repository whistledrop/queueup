package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	s.mux.HandleFunc("POST /api/billing/code", s.withAccount(s.handleCheckCode))
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
	writeError(w, http.StatusPaymentRequired,
		"Joining needs the QueueUp subscription: "+priceLine+", cancel anytime.")
	return false
}

func (s *Server) handleBilling(w http.ResponseWriter, r *http.Request, acct store.Account) {
	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't check your subscription.")
		return
	}
	// There is one public price. The lower first month exists only behind a
	// code, because the code is how we find out which channel a customer came
	// from, and a discount everybody gets automatically tells us nothing.
	code, _ := s.st.SourceCode(acct.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":        s.cfg.BillingEnabled,
		"subscribed":     !s.cfg.BillingEnabled || sub.Active(),
		"paying":         sub.Active(),
		"comped":         sub.Comped(),
		"price_pence":    priceMonthlyPence,
		"price_line":     priceLine,
		"source_code":    code,
		"currency":       priceCurrency,
		"subscribed_at":  sub.SubscribedAt,
		"ends_at":        sub.EndsAt,
		"offer_ends_at":  offerEndsAt(acct),
		"can_manage":     sub.CustomerID != "" && s.cfg.Stripe.Enabled() && !sub.Comped(),
		"checkout_ready": s.stripeReady(),
		"test_mode":      s.cfg.Stripe.TestMode(),
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
	// The code they arrived with or typed. Resolving it here, rather than
	// letting Stripe's checkout page collect one, is what keeps attribution:
	// we need to know which code was used even when it carries no discount.
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body)
	code := strings.ToUpper(strings.TrimSpace(body.Code))
	if code == "" {
		code, _ = s.st.SourceCode(acct.ID)
	}

	ctxCode, cancelCode := context.WithTimeout(r.Context(), 15*time.Second)
	promo, err := s.cfg.Stripe.LookupPromotionCode(ctxCode, code, priceMonthlyPence)
	cancelCode()
	switch {
	case code == "":
		// No code. Standard price, no attribution, which is the cost of a
		// customer arriving with nothing.
	case errors.Is(err, stripe.ErrNoSuchCode):
		writeError(w, http.StatusBadRequest, "That code isn't valid. Leave it blank to subscribe at the standard price.")
		return
	case err != nil:
		s.log.Error("looking up a promo code", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway, "Couldn't check that code. Try again in a moment.")
		return
	case promo.Forever:
		// A never-ending discount is a pricing mistake wearing a code's
		// clothes. Refuse it rather than sell a subscription at a loss forever.
		s.log.Error("refusing a forever discount code", "code", promo.Code)
		writeError(w, http.StatusBadRequest, "That code isn't valid. Leave it blank to subscribe at the standard price.")
		return
	default:
		if err := s.st.RememberSourceCode(acct.ID, promo.Code); err != nil {
			s.log.Error("remembering a source code", "account", acct.ID, "err", err)
		}
		// Some codes belong to a channel and some belong to a person. A
		// person's code owes them something, so it is written down here, once,
		// and never moved afterwards.
		if referrer, err := s.st.AccountForReferralCode(promo.Code); err == nil {
			if err := s.st.RecordReferredBy(acct.ID, referrer); err != nil {
				s.log.Error("recording a referrer", "account", acct.ID, "err", err)
			}
		}
	}

	// The discount has a deadline; the attribution does not. Past the window
	// they can still subscribe with their code, and it still says which video
	// brought them — which is what codes exist for — but the first month is
	// full price. Enforced here, on the server, because a countdown that only
	// lives in the page is a countdown anybody can wind back.
	discount := promo.ID
	if discount != "" && !offerOpen(acct, time.Now()) {
		discount = ""
	}

	web := strings.TrimSuffix(s.cfg.WebURL, "/")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	url, err := s.cfg.Stripe.CreateCheckoutSession(ctx, stripe.CheckoutParams{
		AccountID:  acct.ID,
		Email:      acct.Email,
		CustomerID: sub.CustomerID,
		PriceID:    s.cfg.StripePriceID,

		PromotionCodeID: discount,
		SourceCode:      promo.Code,
		// The home page, which every version of the website has. The new one
		// passes somebody who paid without a password on to choose one; this
		// used to point straight at that step, and for the minutes a website
		// deploy lagged behind the relay, people who had just paid landed on
		// a page that did not exist yet.
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
		fresh := !current.Active() || current.SubID != live.ID
		if fresh {
			s.log.Info("subscription active", "account", accountID, "status", live.Status)
			if err := s.st.SetSubscription(accountID, "active", live.ID); err != nil {
				return err
			}
		}
		// Whether they are staying or on their way out. This comes after any
		// status change on purpose: setting the status clears the leaving
		// date, so doing it the other way round would wipe what we just wrote.
		//
		// Somebody who cancels keeps the month they paid for, so Stripe leaves
		// the status "active" and nothing above this line changes. Without
		// this, cancelling is invisible to us and to them.
		ends := live.EndsAt()
		if !ends.IsZero() && current.EndsAt.IsZero() {
			s.log.Info("subscription will end", "account", accountID, "on", ends.Format("2006-01-02"))
		}
		if ends.IsZero() && !current.EndsAt.IsZero() {
			s.log.Info("cancellation withdrawn", "account", accountID)
		}
		if err := s.st.NoteEnding(accountID, ends); err != nil {
			return err
		}
		if fresh {
			// They have paid. If their PC is already on, whoever sent them has
			// just earned a month.
			s.maybeAwardReferral(ctx, accountID)
			return nil
		}
		// An update on a subscription already active is usually a renewal,
		// which is the moment last month's reward finished being spent and the
		// next one can take its place.
		s.applyReferralCredits(ctx, accountID)
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

// handleCheckCode tells the paywall what a code is actually worth, and
// remembers it.
//
// It exists so that somebody arriving on ?promo=TIKTOK sees "£1.99 for your
// first month" as a real number on the page rather than being asked to trust a
// discount that only appears on Stripe's screen after they have committed.
//
// It also records the code, which is why it runs on arrival and not only at
// checkout: people land today and pay on Thursday, and the attribution has to
// survive the gap.
func (s *Server) handleCheckCode(w http.ResponseWriter, r *http.Request, acct store.Account) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that.")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(body.Code))
	if code == "" {
		writeError(w, http.StatusBadRequest, "Type a code first.")
		return
	}
	if !s.stripeReady() {
		writeError(w, http.StatusNotImplemented, "Codes aren't switched on yet.")
		return
	}
	// Guessing at codes is cheap and the prize is a discount, so it meets the
	// same limit as guessing at passwords.
	from := "code:" + s.clientIP(r)
	if s.signIns.blocked(from) {
		writeError(w, http.StatusTooManyRequests, "Too many codes tried. Wait a few minutes.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	promo, err := s.cfg.Stripe.LookupPromotionCode(ctx, code, priceMonthlyPence)
	if err != nil || promo.Forever {
		if err != nil && !errors.Is(err, stripe.ErrNoSuchCode) {
			s.log.Error("checking a code", "err", err)
			writeError(w, http.StatusBadGateway, "Couldn't check that code. Try again in a moment.")
			return
		}
		s.signIns.fail(from)
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"line":  "That code isn't valid. You can subscribe without one.",
		})
		return
	}
	s.signIns.reset(from)
	if err := s.st.RememberSourceCode(acct.ID, promo.Code); err != nil {
		s.log.Error("remembering a source code", "account", acct.ID, "err", err)
	}
	// A real code past its deadline is still a real code: it is remembered
	// for attribution and reported as valid, but the price it quotes is the
	// price they would actually pay. The page must never show £1.99 for a
	// checkout that will charge £4.99.
	if !offerOpen(acct, time.Now()) {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":             true,
			"expired":           true,
			"code":              promo.Code,
			"first_month_pence": priceMonthlyPence,
			"line":              "That offer has ended. " + priceLine + ", cancel anytime.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":             true,
		"code":              promo.Code,
		"first_month_pence": promo.FirstMonthPence,
		"line":              moneyLine(promo.FirstMonthPence) + " for your first month, then " + priceLine + ".",
	})
}

// offerWindow is how long the discounted first month stays open, counted
// from the moment somebody creates their account — which is the moment they
// first see the price.
const offerWindow = 72 * time.Hour

// offerEndsAt is when this account's first-month offer closes.
func offerEndsAt(acct store.Account) time.Time { return acct.CreatedAt.Add(offerWindow) }

// offerOpen reports whether the discount still applies.
func offerOpen(acct store.Account, now time.Time) bool { return now.Before(offerEndsAt(acct)) }

// moneyLine writes pence the way a price is written on a page.
func moneyLine(pence int) string {
	if pence == 0 {
		return "Free"
	}
	return fmt.Sprintf("£%d.%02d", pence/100, pence%100)
}
