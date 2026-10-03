package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Setup creates everything QueueUp needs inside a Stripe account, so nobody has
// to click through Stripe's dashboard to make a product, a price, a discount, a
// webhook and a customer portal, and get one of them subtly wrong.
//
// It runs once per Stripe mode: once with the test key, once with the live key.

// Plan is the price QueueUp charges.
type Plan struct {
	Name           string
	Currency       string // lower case, e.g. "gbp"
	MonthlyPence   int64  // e.g. 499
	IntroPence     int64  // what the first month costs, e.g. 199
	WebhookURL     string // e.g. https://queueup-relay.fly.dev/stripe/webhook
	PortalReturn   string // where "back" goes from Stripe's manage page
	TermsOfService string
}

// Created is what Setup made, to be stored as the relay's settings.
type Created struct {
	ProductID     string
	PriceID       string
	IntroCouponID string
	// ReferralCouponID brings one month down to the intro price, as thanks for
	// bringing somebody who actually turned up and paid.
	ReferralCouponID string
	WebhookSecret    string
	PortalConfig     string
}

// WebhookEvents are the only events QueueUp listens to.
var WebhookEvents = []string{
	"checkout.session.completed",
	"customer.subscription.created",
	"customer.subscription.updated",
	"customer.subscription.deleted",
}

// Setup makes the product, the monthly price, the first-month discount, the
// webhook endpoint and the customer portal.
func (c *Client) Setup(ctx context.Context, p Plan) (Created, error) {
	var out Created
	var obj struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}

	f := url.Values{}
	f.Set("name", p.Name)
	f.Set("description", "Join Rust servers from your phone while your PC queues.")
	if err := c.call(ctx, http.MethodPost, "/v1/products", f, &obj); err != nil {
		return out, fmt.Errorf("creating the product: %w", err)
	}
	out.ProductID = obj.ID

	f = url.Values{}
	f.Set("product", out.ProductID)
	f.Set("currency", p.Currency)
	f.Set("unit_amount", strconv.FormatInt(p.MonthlyPence, 10))
	f.Set("recurring[interval]", "month")
	f.Set("nickname", "Monthly")
	if err := c.call(ctx, http.MethodPost, "/v1/prices", f, &obj); err != nil {
		return out, fmt.Errorf("creating the price: %w", err)
	}
	out.PriceID = obj.ID

	// The intro offer is a discount on the first invoice only: £4.99 minus
	// £3.00 is £1.99, and Stripe charges the full price from month two
	// without anybody doing anything.
	off := p.MonthlyPence - p.IntroPence
	if off > 0 {
		f = url.Values{}
		f.Set("amount_off", strconv.FormatInt(off, 10))
		f.Set("currency", p.Currency)
		f.Set("duration", "once")
		f.Set("name", "First month")
		if err := c.call(ctx, http.MethodPost, "/v1/coupons", f, &obj); err != nil {
			return out, fmt.Errorf("creating the first-month discount: %w", err)
		}
		out.IntroCouponID = obj.ID

		// The same money off, for a different reason. A separate coupon so the
		// books can tell "first month offer" apart from "referred a mate",
		// which is the difference between a marketing cost and a growth one.
		f = url.Values{}
		f.Set("amount_off", strconv.FormatInt(off, 10))
		f.Set("currency", p.Currency)
		f.Set("duration", "once")
		f.Set("name", "Referral month")
		if err := c.call(ctx, http.MethodPost, "/v1/coupons", f, &obj); err != nil {
			return out, fmt.Errorf("creating the referral discount: %w", err)
		}
		out.ReferralCouponID = obj.ID
	}

	f = url.Values{}
	f.Set("url", p.WebhookURL)
	f.Set("description", "QueueUp relay")
	for i, e := range WebhookEvents {
		f.Set(fmt.Sprintf("enabled_events[%d]", i), e)
	}
	if err := c.call(ctx, http.MethodPost, "/v1/webhook_endpoints", f, &obj); err != nil {
		return out, fmt.Errorf("creating the webhook: %w", err)
	}
	out.WebhookSecret = obj.Secret

	// The portal is Stripe's page for changing card and cancelling. Cancelling
	// takes effect at the end of the paid month, so nobody loses days they
	// have already paid for.
	f = url.Values{}
	f.Set("business_profile[headline]", "Manage your QueueUp subscription")
	if p.TermsOfService != "" {
		f.Set("business_profile[terms_of_service_url]", p.TermsOfService)
	}
	f.Set("default_return_url", p.PortalReturn)
	f.Set("features[customer_update][enabled]", "true")
	f.Set("features[customer_update][allowed_updates][0]", "email")
	f.Set("features[invoice_history][enabled]", "true")
	f.Set("features[payment_method_update][enabled]", "true")
	f.Set("features[subscription_cancel][enabled]", "true")
	f.Set("features[subscription_cancel][mode]", "at_period_end")
	if err := c.call(ctx, http.MethodPost, "/v1/billing_portal/configurations", f, &obj); err != nil {
		return out, fmt.Errorf("setting up the manage page: %w", err)
	}
	out.PortalConfig = obj.ID
	return out, nil
}

// CreatePromotionCode mints one customer-facing code for a channel.
//
// Under the pricing we actually run, the discounted first month exists ONLY
// behind a code, and every channel gets its own: TIKTOK, YOUTUBE, a server's
// own name. That is not merely a discount mechanism, it is the only way we
// ever find out where a paying customer came from, which is what decides
// where the next month's effort goes.
func (c *Client) CreatePromotionCode(ctx context.Context, couponID, code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if couponID == "" || code == "" {
		return "", errors.New("a coupon and a code are both needed")
	}
	f := url.Values{}
	f.Set("coupon", couponID)
	f.Set("code", code)
	var obj struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/promotion_codes", f, &obj); err != nil {
		return "", err
	}
	return obj.Code, nil
}
