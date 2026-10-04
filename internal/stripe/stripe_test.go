package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The webhook is how an account becomes "paid". Every way of faking one must
// fail, or the subscription is free to anybody who can send a request.
func TestOnlyGenuineRecentWebhooksAreBelieved(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{}}}`)
	now := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)

	if _, err := VerifyWebhook(body, SignForTests(body, secret, now), secret, now); err != nil {
		t.Fatalf("a genuine webhook was refused: %v", err)
	}

	cases := map[string]string{
		"no signature":    "",
		"wrong secret":    SignForTests(body, "whsec_other", now),
		"too old":         SignForTests(body, secret, now.Add(-10*time.Minute)),
		"from the future": SignForTests(body, secret, now.Add(10*time.Minute)),
		"garbage":         "t=abc,v1=zzz",
		"signature only":  "v1=" + strings.Repeat("a", 64),
	}
	for name, header := range cases {
		if _, err := VerifyWebhook(body, header, secret, now); err == nil {
			t.Errorf("%s: a fake webhook was believed", name)
		}
	}

	// The body changed after signing: somebody edited a real webhook.
	tampered := []byte(strings.Replace(string(body), "evt_1", "evt_2", 1))
	if _, err := VerifyWebhook(tampered, SignForTests(body, secret, now), secret, now); err == nil {
		t.Error("an edited webhook was believed")
	}
	// No secret configured must never mean "anything goes".
	if _, err := VerifyWebhook(body, SignForTests(body, "", now), "", now); err == nil {
		t.Error("with no secret configured, a webhook was believed")
	}
}

func TestWhichStatusesOpenTheGate(t *testing.T) {
	open := map[string]bool{
		"active": true, "trialing": true, "past_due": true,
		"canceled": false, "unpaid": false, "incomplete": false,
		"incomplete_expired": false, "paused": false, "": false,
	}
	for status, want := range open {
		if got := GrantsAccess(status); got != want {
			t.Errorf("GrantsAccess(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestTestKeysAreRecognised(t *testing.T) {
	if !(&Client{SecretKey: "sk_test_abc"}).TestMode() {
		t.Error("a test key was not recognised as test mode")
	}
	if (&Client{SecretKey: "sk_live_abc"}).TestMode() {
		t.Error("a live key was taken for test mode")
	}
	if (&Client{}).Enabled() {
		t.Error("a client with no key claims to be connected")
	}
}

// A promotion code is read back from Stripe in the shape Stripe actually
// sends, which changed under us.
//
// Until API version 2026-08-26 a promotion code carried its coupon in a
// top-level "coupon" field. It now carries it in "promotion", as an id. The
// old parser read the missing field, got a zero coupon whose "valid" was
// false, and reported every code on the account as invalid: TIKTOK, the
// channel codes, and every referral code anybody had ever earned. The paywall
// said "that code isn't valid" about codes that were perfectly valid.
//
// So this serves the documented current shape, and the old one beside it,
// because an account pinned to an older version must keep working.
func TestACodeIsReadFromWhereStripeActuallyPutsIt(t *testing.T) {
	const fullPrice = 499

	cases := []struct {
		name     string
		promoRow string
		// wantCouponFetch is true when the id-only shape forces the second
		// call: a code is worthless if we cannot say what it takes off.
		wantCouponFetch bool
	}{{
		name: "current shape, coupon as an id under promotion",
		promoRow: `{"id":"promo_live","code":"TIKTOK","active":true,
		            "promotion":{"type":"coupon","coupon":"mCe3PXBt"}}`,
		wantCouponFetch: true,
	}, {
		name: "older shape, coupon as an id at the top level",
		promoRow: `{"id":"promo_live","code":"TIKTOK","active":true,
		            "coupon":"mCe3PXBt"}`,
		wantCouponFetch: true,
	}, {
		name: "coupon already expanded, so no second call is needed",
		promoRow: `{"id":"promo_live","code":"TIKTOK","active":true,
		            "promotion":{"type":"coupon","coupon":
		              {"id":"mCe3PXBt","amount_off":300,"currency":"gbp",
		               "duration":"once","valid":true}}}`,
		wantCouponFetch: false,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var askedForCoupon bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/promotion_codes":
					if got := r.URL.Query().Get("code"); got != "TIKTOK" {
						t.Errorf("asked Stripe for code %q, want TIKTOK", got)
					}
					// Expanding a field an account's API version does not
					// have is a 400, which would turn a working code into a
					// server error. We must not ask for one.
					if exp := r.URL.Query()["expand[]"]; len(exp) > 0 {
						t.Errorf("asked Stripe to expand %v; fetch the coupon instead", exp)
					}
					fmt.Fprintf(w, `{"data":[%s]}`, c.promoRow)
				case r.URL.Path == "/v1/coupons/mCe3PXBt":
					askedForCoupon = true
					fmt.Fprint(w, `{"id":"mCe3PXBt","amount_off":300,"currency":"gbp",
					                "duration":"once","valid":true}`)
				default:
					t.Errorf("unexpected call to %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			cl := &Client{SecretKey: "sk_test_fake", BaseURL: srv.URL}
			got, err := cl.LookupPromotionCode(context.Background(), "tiktok", fullPrice)
			if err != nil {
				t.Fatalf("a valid code was rejected: %v", err)
			}
			if got.Code != "TIKTOK" || got.ID != "promo_live" {
				t.Errorf("code = %+v, want TIKTOK/promo_live", got)
			}
			// £4.99 less £3.00 is the £1.99 first month the page promises.
			if got.FirstMonthPence != 199 {
				t.Errorf("first month = %dp, want 199p", got.FirstMonthPence)
			}
			if got.Forever {
				t.Error("a once-only discount was read as lasting forever")
			}
			if askedForCoupon != c.wantCouponFetch {
				t.Errorf("fetched the coupon = %v, want %v", askedForCoupon, c.wantCouponFetch)
			}
		})
	}
}

// The checks that stop a bad code, or a code that would cost us money every
// month, from reaching a checkout.
func TestCodesThatMustNotBeAccepted(t *testing.T) {
	cases := []struct {
		name      string
		promoList string
		coupon    string
		wantErr   bool
		wantEvery bool // valid, but the discount never ends
	}{{
		name:      "no such code",
		promoList: `{"data":[]}`,
		wantErr:   true,
	}, {
		name:      "the code exists but has been switched off",
		promoList: `{"data":[{"id":"p","code":"OLD","active":false,"promotion":{"type":"coupon","coupon":"c"}}]}`,
		wantErr:   true,
	}, {
		name:      "the coupon behind it is no longer valid",
		promoList: `{"data":[{"id":"p","code":"SPENT","active":true,"promotion":{"type":"coupon","coupon":"c"}}]}`,
		coupon:    `{"id":"c","amount_off":300,"duration":"once","valid":false}`,
		wantErr:   true,
	}, {
		name:      "a promotion that is not a coupon at all",
		promoList: `{"data":[{"id":"p","code":"ODD","active":true,"promotion":{"type":"something_new"}}]}`,
		wantErr:   true,
	}, {
		name:      "a discount that never stops is reported, not hidden",
		promoList: `{"data":[{"id":"p","code":"OOPS","active":true,"promotion":{"type":"coupon","coupon":"c"}}]}`,
		coupon:    `{"id":"c","percent_off":100,"duration":"forever","valid":true}`,
		wantEvery: true,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/coupons/") {
					if c.coupon == "" {
						t.Errorf("fetched a coupon it had no id for")
						w.WriteHeader(http.StatusNotFound)
						return
					}
					fmt.Fprint(w, c.coupon)
					return
				}
				fmt.Fprint(w, c.promoList)
			}))
			defer srv.Close()

			cl := &Client{SecretKey: "sk_test_fake", BaseURL: srv.URL}
			got, err := cl.LookupPromotionCode(context.Background(), "ANY", 499)
			if c.wantErr {
				if !errors.Is(err, ErrNoSuchCode) {
					t.Fatalf("err = %v, want ErrNoSuchCode", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Forever != c.wantEvery {
				t.Errorf("Forever = %v, want %v", got.Forever, c.wantEvery)
			}
		})
	}
}

// A referral month is given by putting a coupon on a live subscription, and
// the parameter for that moved too.
//
// Stripe rejects a parameter it does not recognise instead of ignoring it, so
// the old top-level "coupon" did not quietly do nothing: it failed the call.
// Every referral month anybody earned would have been spent from our side and
// never applied on Stripe's.
func TestAReferralMonthIsSentTheWayStripeTakesItNow(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parsing the form: %v", err)
		}
		got = r.PostForm
		fmt.Fprint(w, `{"id":"sub_live"}`)
	}))
	defer srv.Close()

	cl := &Client{SecretKey: "sk_test_fake", BaseURL: srv.URL}
	if err := cl.ApplyCoupon(context.Background(), "sub_live", "90rfYHGO"); err != nil {
		t.Fatalf("ApplyCoupon: %v", err)
	}
	if v := got.Get("discounts[0][coupon]"); v != "90rfYHGO" {
		t.Errorf("discounts[0][coupon] = %q, want the coupon id", v)
	}
	if got.Has("coupon") {
		t.Error("still sending the top-level coupon Stripe removed")
	}

	// Both arguments are required, because a call with either missing would
	// ask Stripe to clear the subscription's discounts.
	if err := cl.ApplyCoupon(context.Background(), "", "c"); err == nil {
		t.Error("applied a coupon to no subscription")
	}
	if err := cl.ApplyCoupon(context.Background(), "sub_live", ""); err == nil {
		t.Error("applied an empty coupon, which would clear the real one")
	}
}

// Whether a month is already waiting decides whether the next one is handed
// over or kept, and a wrong answer here spends a credit for nothing: applying
// a discount replaces the one already on the subscription.
func TestADiscountAlreadyOnTheSubscriptionIsSeen(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"current shape, a discount id in the list", `{"discounts":["di_1"]}`, true},
		{"current shape, an expanded discount in the list",
			`{"discounts":[{"id":"di_1","coupon":{"id":"90rfYHGO"}}]}`, true},
		{"current shape, nothing waiting", `{"discounts":[]}`, false},
		{"current shape, explicitly null", `{"discounts":null}`, false},
		{"older shape, a discount", `{"discount":{"coupon":{"id":"90rfYHGO"}}}`, true},
		{"older shape, nothing waiting", `{"discount":null}`, false},
		{"neither field sent at all", `{"id":"sub_live","status":"active"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, c.body)
			}))
			defer srv.Close()

			cl := &Client{SecretKey: "sk_test_fake", BaseURL: srv.URL}
			sub, err := cl.GetSubscription(context.Background(), "sub_live")
			if err != nil {
				t.Fatalf("GetSubscription: %v", err)
			}
			if sub.HasDiscount() != c.want {
				t.Errorf("HasDiscount() = %v, want %v", sub.HasDiscount(), c.want)
			}
		})
	}
}

// When a cancelled subscription actually stops.
//
// Stripe reports this in more than one place and not always in the same one:
// cancel_at is filled in when a date was named outright, but cancelling at the
// end of the period does not reliably set it, and the period's end has moved
// off the subscription onto its items. A date shown to somebody is a date they
// plan a wipe around, so a wrong one is worse than none.
func TestWhenACancelledSubscriptionActuallyStops(t *testing.T) {
	day := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	item := func(ts ...time.Time) string {
		parts := make([]string, len(ts))
		for i, at := range ts {
			parts[i] = fmt.Sprintf(`{"current_period_end":%d}`, at.Unix())
		}
		return `{"items":{"data":[` + strings.Join(parts, ",") + `]}`
	}

	cases := []struct {
		name string
		body string
		want time.Time
	}{{
		name: "not cancelled, so no end date however much else is set",
		body: item(day) + `,"cancel_at_period_end":false}`,
	}, {
		name: "cancelled at period end: the date is on the item",
		body: item(day) + `,"cancel_at_period_end":true}`,
		want: day,
	}, {
		name: "a named cancellation date wins",
		body: fmt.Sprintf(`{"cancel_at":%d,"cancel_at_period_end":false}`, day.Unix()),
		want: day,
	}, {
		name: "several items: the last one to end is when access really stops",
		body: item(day.AddDate(0, 0, -7), day, day.AddDate(0, 0, -3)) + `,"cancel_at_period_end":true}`,
		want: day,
	}, {
		name: "older shape, the period end still on the subscription",
		body: fmt.Sprintf(`{"cancel_at_period_end":true,"current_period_end":%d}`, day.Unix()),
		want: day,
	}, {
		name: "cancelled but Stripe gave us no date: say nothing rather than guess",
		body: `{"cancel_at_period_end":true}`,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sub Subscription
			if err := json.Unmarshal([]byte(c.body), &sub); err != nil {
				t.Fatalf("unmarshalling: %v", err)
			}
			if got := sub.EndsAt(); !got.Equal(c.want) {
				t.Errorf("EndsAt() = %v, want %v", got, c.want)
			}
		})
	}
}
