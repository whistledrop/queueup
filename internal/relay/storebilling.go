package relay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"queueup/internal/revenuecat"
	"queueup/internal/store"
)

// Subscriptions bought in the phone app, through the App Store and Google Play.
//
// Apple and Google both require a subscription used inside an app to be sold
// inside it, so the app sells Unlimited through them, by way of RevenueCat. A
// store subscription opens exactly the same gate a Stripe one does: the
// account's status goes "active", and the free plan's limits stop applying.
//
// Two ways news arrives, one way it is acted on:
//   - RevenueCat's webhook, for renewals, cancellations and expiries that
//     happen while nobody is looking at the app;
//   - the app itself, straight after a purchase or a restore, so Unlimited is
//     open before the person has finished reading "thank you".
//
// Neither is trusted for WHAT changed. Both just say whose subscription to
// look at, and the relay asks RevenueCat what is true now. Same reason as
// Stripe: webhooks arrive late and out of order.
//
// A store subscription is recorded with a subscription id of "<store>:<product>",
// which is what keeps the two payment routes from switching each other off.
// Stripe only ever closes the subscription it opened, and the stores only ever
// close one of theirs.

func (s *Server) storeBillingRoutes() {
	s.mux.HandleFunc("POST /revenuecat/webhook", s.handleRevenueCatWebhook)
	s.mux.HandleFunc("POST /api/billing/store-sync", s.withAccount(s.handleStoreSync))
}

// storeSubID is how a store subscription is written on the account.
func storeSubID(st revenuecat.Status) string {
	store := st.Store
	if store == "" {
		store = "store"
	}
	return store + ":" + st.Product
}

// storeOf says which store a subscription id belongs to, or "" for one that
// is not a store's (Stripe's, a hand-given one, or none).
func storeOf(subID string) string {
	for _, st := range []string{revenuecat.AppStore, revenuecat.PlayStore, "store"} {
		if strings.HasPrefix(subID, st+":") {
			return st
		}
	}
	return ""
}

// managedBy tells the website and the app where this subscription is changed
// or cancelled, which is never QueueUp's own screens for a store purchase:
// Apple and Google keep that to themselves.
func managedBy(sub store.Subscription) string {
	switch {
	case !sub.Active():
		return ""
	case sub.Comped():
		return "comped"
	case storeOf(sub.SubID) != "":
		return storeOf(sub.SubID)
	default:
		return "stripe"
	}
}

// syncStore copies an account's CURRENT store subscription from RevenueCat.
func (s *Server) syncStore(ctx context.Context, accountID string) error {
	live, err := s.cfg.RevenueCat.Customer(ctx, accountID, time.Now())
	if err != nil {
		return err
	}
	current, err := s.st.SubscriptionFor(accountID)
	if err != nil {
		return err
	}

	if live.Active {
		id := storeSubID(live)
		if current.Active() && storeOf(current.SubID) == "" {
			// Already paying on the website, or given access by hand, and
			// now subscribed in the app as well. The gate is open either way,
			// so nothing on the account changes; but somebody is paying twice
			// and will want one of them refunded, so it is said loudly.
			s.log.Warn("a store subscription on an account that already has access another way",
				"account", accountID, "store", live.Store, "existing", managedBy(current))
			return nil
		}
		fresh := !current.Active() || current.SubID != id
		if fresh {
			s.log.Info("store subscription active", "account", accountID, "store", live.Store)
			if err := s.st.SetSubscription(accountID, "active", id); err != nil {
				return err
			}
		}
		// After the status, because setting the status clears the leaving date.
		var ends time.Time
		if live.Ending {
			ends = live.ExpiresAt
		}
		if !ends.IsZero() && current.EndsAt.IsZero() {
			s.log.Info("store subscription will end", "account", accountID, "on", ends.Format("2006-01-02"))
		}
		return s.st.NoteEnding(accountID, ends)
	}

	// Closed in the store. Only a store subscription can be closed by one: a
	// Stripe subscription or a hand-given month is none of Apple's business.
	if storeOf(current.SubID) == "" {
		return nil
	}
	if current.Active() {
		s.log.Info("store subscription ended", "account", accountID)
	}
	return s.st.SetSubscription(accountID, "none", current.SubID)
}

// handleRevenueCatWebhook hears about renewals, cancellations and expiries.
//
// RevenueCat retries anything that is not a 2xx, so: a request without the
// secret is refused, an event about somebody QueueUp does not know is accepted
// (retrying will not make them exist), and only a failure retrying could fix
// gets a 500.
func (s *Server) handleRevenueCatWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that.")
		return
	}
	ev, err := revenuecat.ReadWebhook(body, r.Header.Get("Authorization"), s.cfg.RevenueCatWebhookSecret)
	if errors.Is(err, revenuecat.ErrUnauthorised) {
		s.log.Warn("refused a webhook that did not come from RevenueCat")
		writeError(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that.")
		return
	}
	if !s.cfg.RevenueCat.Enabled() {
		// Told about a purchase with no way to look it up. Asking RevenueCat
		// to come back later is right: the key may be on its way.
		s.log.Error("a RevenueCat webhook arrived but QUEUEUP_REVENUECAT_SECRET_KEY is not set", "type", ev.Type)
		writeError(w, http.StatusServiceUnavailable, "Not ready.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	known := 0
	for _, id := range ev.AccountIDs {
		if _, err := s.st.AccountByID(id); err != nil {
			continue
		}
		known++
		if err := s.syncStore(ctx, id); err != nil {
			s.log.Error("reading a store subscription from RevenueCat", "account", id, "type", ev.Type, "err", err)
			writeError(w, http.StatusInternalServerError, "Try again.")
			return
		}
	}
	if known == 0 && ev.Type != "TEST" {
		s.log.Warn("a RevenueCat event for nobody QueueUp knows", "type", ev.Type)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStoreSync is the app saying "I have just bought or restored". It
// answers with the same billing summary as GET /api/billing, now up to date,
// so the app can show Unlimited without a second request.
func (s *Server) handleStoreSync(w http.ResponseWriter, r *http.Request, acct store.Account) {
	if !s.cfg.RevenueCat.Enabled() {
		writeError(w, http.StatusNotImplemented, "In-app subscriptions aren't switched on yet.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.syncStore(ctx, acct.ID); err != nil {
		s.log.Error("syncing a store subscription", "account", acct.ID, "err", err)
		writeError(w, http.StatusBadGateway, "Couldn't check your purchase with the store. Try Restore purchases in a moment.")
		return
	}
	s.handleBilling(w, r, acct)
}
