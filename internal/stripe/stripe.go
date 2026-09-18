// Package stripe is the small part of Stripe that QueueUp uses: a hosted
// checkout page, Stripe's own "manage my subscription" page, reading a
// subscription, and checking that a webhook really came from Stripe.
//
// It is plain HTTPS rather than Stripe's SDK, for the same reason the email
// sender is: four calls do not justify a dependency, and every line here can
// be read and tested.
//
// Card details never touch QueueUp. People type them on Stripe's page, and
// QueueUp only ever hears "this account's subscription is now active" or "is
// now over".
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to Stripe with a secret key. A nil or keyless client is
// disabled, which is how the relay runs locally and in tests.
type Client struct {
	SecretKey string
	// BaseURL is Stripe's API, overridden in tests.
	BaseURL string
	HTTP    *http.Client
}

// Enabled reports whether there is a key to talk to Stripe with.
func (c *Client) Enabled() bool { return c != nil && c.SecretKey != "" }

// TestMode reports whether the key is a test key: no real money moves.
func (c *Client) TestMode() bool {
	return c.Enabled() && (strings.HasPrefix(c.SecretKey, "sk_test_") || strings.HasPrefix(c.SecretKey, "rk_test_"))
}

// Error is Stripe refusing a request, in its own words.
type Error struct {
	Status  int
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return "Stripe: " + e.Message
	}
	return fmt.Sprintf("Stripe returned %d", e.Status)
}

func (c *Client) call(ctx context.Context, method, path string, form url.Values, out any) error {
	if !c.Enabled() {
		return errors.New("Stripe is not connected")
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.stripe.com"
	}
	var body io.Reader
	u := strings.TrimSuffix(base, "/") + path
	if method == http.MethodGet {
		if len(form) > 0 {
			u += "?" + form.Encode()
		}
	} else {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.SecretKey, "")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach Stripe: %s", c.scrub(err.Error()))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var wrapped struct {
			Error Error `json:"error"`
		}
		_ = json.Unmarshal(raw, &wrapped)
		e := wrapped.Error
		e.Status = resp.StatusCode
		e.Message = c.scrub(e.Message)
		return &e
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// scrub keeps the secret key out of anything that might reach a log.
func (c *Client) scrub(s string) string {
	if c.SecretKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.SecretKey, "REDACTED")
}

// CheckoutParams is one "subscribe" button press.
type CheckoutParams struct {
	AccountID  string
	Email      string
	CustomerID string // reuse the Stripe customer if this account already has one
	PriceID    string
	CouponID   string // the first-month discount; empty when not eligible
	SuccessURL string
	CancelURL  string
}

// CreateCheckoutSession returns the address of Stripe's payment page.
func (c *Client) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (string, error) {
	f := url.Values{}
	f.Set("mode", "subscription")
	f.Set("line_items[0][price]", p.PriceID)
	f.Set("line_items[0][quantity]", "1")
	f.Set("success_url", p.SuccessURL)
	f.Set("cancel_url", p.CancelURL)
	// Both of these carry the account id, so every webhook can be matched to
	// the right person without trusting anything the browser said.
	f.Set("client_reference_id", p.AccountID)
	f.Set("subscription_data[metadata][account_id]", p.AccountID)
	if p.CustomerID != "" {
		f.Set("customer", p.CustomerID)
	} else if p.Email != "" {
		f.Set("customer_email", p.Email)
	}
	if p.CouponID != "" {
		f.Set("discounts[0][coupon]", p.CouponID)
	} else {
		// Without the intro discount applied, people may still have a code
		// from a creator or a giveaway.
		f.Set("allow_promotion_codes", "true")
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/checkout/sessions", f, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("Stripe did not return a checkout page")
	}
	return out.URL, nil
}

// CreatePortalSession returns the address of Stripe's own page for changing
// card, seeing invoices and cancelling.
func (c *Client) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	f := url.Values{}
	f.Set("customer", customerID)
	f.Set("return_url", returnURL)
	var out struct {
		URL string `json:"url"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/billing_portal/sessions", f, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Subscription is what QueueUp needs to know about one Stripe subscription.
type Subscription struct {
	ID       string            `json:"id"`
	Status   string            `json:"status"`
	Customer string            `json:"customer"`
	Metadata map[string]string `json:"metadata"`
}

// GetSubscription reads a subscription as Stripe has it now. Webhooks can
// arrive late and out of order; asking for the current state instead of
// trusting the order they came in is what keeps the local copy right.
func (c *Client) GetSubscription(ctx context.Context, id string) (Subscription, error) {
	var s Subscription
	err := c.call(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(id), nil, &s)
	return s, err
}

// GrantsAccess says whether a Stripe subscription status should open the gate.
//
// past_due stays open on purpose: it means a renewal payment failed and Stripe
// is retrying, usually because a card expired. Locking somebody out on wipe
// day over a card they will fix tomorrow loses the customer; Stripe ends the
// subscription itself if the retries all fail.
func GrantsAccess(status string) bool {
	switch status {
	case "active", "trialing", "past_due":
		return true
	}
	return false
}

// ---------------------------------------------------------------- webhooks

// Event is a webhook, as much of it as QueueUp reads.
type Event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// CheckoutSession is the object inside checkout.session.completed.
type CheckoutSession struct {
	ClientReferenceID string `json:"client_reference_id"`
	Customer          string `json:"customer"`
	Subscription      string `json:"subscription"`
	Mode              string `json:"mode"`
}

// WebhookTolerance is how old a signed webhook may be. Stripe's own default.
// It stops somebody who captured one real webhook replaying it later.
const WebhookTolerance = 5 * time.Minute

// ErrBadSignature covers every reason a webhook is refused.
var ErrBadSignature = errors.New("webhook signature is not valid")

// VerifyWebhook checks that a webhook really came from Stripe, then reads it.
//
// This is the whole of QueueUp's billing security: the webhook is how an
// account becomes "paid", so a webhook that is not checked is a free
// subscription for anybody who can send an HTTP request.
func VerifyWebhook(payload []byte, header, secret string, now time.Time) (Event, error) {
	if secret == "" {
		return Event{}, errors.New("no webhook secret configured")
	}
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return Event{}, ErrBadSignature
	}
	age := now.Sub(time.Unix(ts, 0))
	if age > WebhookTolerance || age < -WebhookTolerance {
		return Event{}, ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := mac.Sum(nil)

	valid := false
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			valid = true
		}
	}
	if !valid {
		return Event{}, ErrBadSignature
	}
	var ev Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return Event{}, fmt.Errorf("webhook is not valid JSON: %w", err)
	}
	return ev, nil
}

// SignForTests produces a valid signature header, so tests can send webhooks
// the relay will believe. Nothing outside tests calls it.
func SignForTests(payload []byte, secret string, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(payload)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
