// Package revenuecat is the small part of RevenueCat that QueueUp uses: asking
// what a customer is subscribed to through the App Store or Google Play, and
// checking that a webhook really came from RevenueCat.
//
// RevenueCat sits between the phone app and both stores. The app buys through
// Apple or Google, RevenueCat checks the receipt with them, and QueueUp only
// ever asks RevenueCat one question: does this account have Unlimited right
// now, from which store, and is it on its way out. Same reasoning as the
// Stripe package: one call does not justify an SDK.
//
// The customer's RevenueCat id is their QueueUp account id. The app logs in to
// RevenueCat with it before it sells anything, so every purchase arrives here
// already knowing whose it is.
package revenuecat

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Entitlement is what a subscription unlocks, as named in RevenueCat. Both
// stores' products are attached to it there, so the relay never needs to know
// product ids.
const Entitlement = "unlimited"

// Client talks to RevenueCat with a secret API key. A keyless client is
// disabled, which is how the relay runs locally and in tests.
type Client struct {
	SecretKey string
	// BaseURL is RevenueCat's API, overridden in tests.
	BaseURL string
	HTTP    *http.Client
}

// Enabled reports whether there is a key to talk to RevenueCat with.
func (c *Client) Enabled() bool { return c != nil && c.SecretKey != "" }

// Store names as RevenueCat writes them.
const (
	AppStore  = "app_store"
	PlayStore = "play_store"
)

// Status is one customer's Unlimited, as the stores currently see it.
type Status struct {
	// Active is whether Unlimited is open right now.
	Active bool
	// Store is where it was bought: AppStore or PlayStore, or something else
	// RevenueCat supports that QueueUp does not sell through.
	Store string
	// Product is the store's product id.
	Product string
	// ExpiresAt is when the current period ends.
	ExpiresAt time.Time
	// Ending means they turned off auto-renew: Unlimited stays open until
	// ExpiresAt and then stops.
	Ending bool
	// BillingIssue means the store could not take the renewal and is retrying.
	BillingIssue bool
}

// Customer asks RevenueCat what this account has right now.
func (c *Client) Customer(ctx context.Context, appUserID string, now time.Time) (Status, error) {
	if !c.Enabled() {
		return Status{}, errors.New("RevenueCat is not connected")
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.revenuecat.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(base, "/")+"/v1/subscribers/"+url.PathEscape(appUserID), nil)
	if err != nil {
		return Status{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Status{}, err
	}
	if res.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return Status{}, fmt.Errorf("RevenueCat: %s", e.Message)
		}
		return Status{}, fmt.Errorf("RevenueCat returned %d", res.StatusCode)
	}
	return parseCustomer(body, now)
}

// parseCustomer reads RevenueCat's answer. Kept apart from the call so the
// rules can be tested on real response shapes without a network.
func parseCustomer(body []byte, now time.Time) (Status, error) {
	var doc struct {
		Subscriber struct {
			Entitlements map[string]struct {
				ExpiresDate       *time.Time `json:"expires_date"`
				ProductIdentifier string     `json:"product_identifier"`
			} `json:"entitlements"`
			Subscriptions map[string]struct {
				Store                   string     `json:"store"`
				ExpiresDate             *time.Time `json:"expires_date"`
				UnsubscribeDetectedAt   *time.Time `json:"unsubscribe_detected_at"`
				BillingIssuesDetectedAt *time.Time `json:"billing_issues_detected_at"`
			} `json:"subscriptions"`
		} `json:"subscriber"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Status{}, fmt.Errorf("reading RevenueCat's answer: %w", err)
	}
	ent, ok := doc.Subscriber.Entitlements[Entitlement]
	if !ok {
		return Status{}, nil // never had it
	}
	st := Status{Product: ent.ProductIdentifier}
	if ent.ExpiresDate != nil {
		st.ExpiresAt = ent.ExpiresDate.UTC()
	}
	// No expiry is a lifetime grant. QueueUp sells nothing like that, but it
	// is RevenueCat's word for "open", so it is taken as such.
	st.Active = ent.ExpiresDate == nil || now.Before(*ent.ExpiresDate)
	if sub, ok := doc.Subscriber.Subscriptions[ent.ProductIdentifier]; ok {
		st.Store = sub.Store
		st.Ending = sub.UnsubscribeDetectedAt != nil
		st.BillingIssue = sub.BillingIssuesDetectedAt != nil
	}
	return st, nil
}

// Event is the part of a RevenueCat webhook QueueUp needs: whose subscription
// changed. What changed is never taken from the webhook itself; the relay asks
// RevenueCat for the current state instead, because webhooks arrive late and
// out of order and a cancellation can land after the renewal that undid it.
type Event struct {
	Type string
	// AccountIDs is every customer id the event names. A transfer, when a
	// restore moves a purchase from one account to another, names both.
	AccountIDs []string
}

// ErrUnauthorised means the webhook did not carry the shared secret.
var ErrUnauthorised = errors.New("webhook did not come from RevenueCat")

// ReadWebhook checks the secret RevenueCat was told to send and reads the
// event. RevenueCat sends the secret as the whole Authorization header,
// exactly as it was typed into its dashboard.
func ReadWebhook(body []byte, authorization, secret string) (Event, error) {
	if secret == "" || subtle.ConstantTimeCompare([]byte(authorization), []byte(secret)) != 1 {
		return Event{}, ErrUnauthorised
	}
	var doc struct {
		Event struct {
			Type            string   `json:"type"`
			AppUserID       string   `json:"app_user_id"`
			OriginalAppUser string   `json:"original_app_user_id"`
			Aliases         []string `json:"aliases"`
			TransferredFrom []string `json:"transferred_from"`
			TransferredTo   []string `json:"transferred_to"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Event{}, fmt.Errorf("reading the webhook: %w", err)
	}
	ev := Event{Type: doc.Event.Type}
	seen := map[string]bool{}
	for _, list := range [][]string{
		{doc.Event.AppUserID, doc.Event.OriginalAppUser},
		doc.Event.Aliases, doc.Event.TransferredFrom, doc.Event.TransferredTo,
	} {
		for _, id := range list {
			// RevenueCat's own ids for somebody not yet logged in are no use
			// here: QueueUp has no account by that name.
			if id == "" || strings.HasPrefix(id, "$RCAnonymousID:") || seen[id] {
				continue
			}
			seen[id] = true
			ev.AccountIDs = append(ev.AccountIDs, id)
		}
	}
	return ev, nil
}
