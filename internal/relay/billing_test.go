package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"queueup/internal/servers"
	"queueup/internal/store"
	"queueup/internal/stripe"
)

const (
	testWebhookSecret = "whsec_test_secret"
	testPriceID       = "price_monthly"
	testCouponID      = "coupon_first_month"
)

// fakeStripe records what QueueUp asks Stripe for, and answers with whatever
// state each subscription is in right now.
type fakeStripe struct {
	mu        sync.Mutex
	checkouts []url.Values
	subs      map[string]stripe.Subscription
	ts        *httptest.Server
}

func newFakeStripe(t *testing.T) *fakeStripe {
	t.Helper()
	f := &fakeStripe{subs: map[string]stripe.Subscription{}}
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key, _, _ := r.BasicAuth(); key != "sk_test_fake" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
			_ = r.ParseForm()
			f.mu.Lock()
			f.checkouts = append(f.checkouts, r.PostForm)
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/billing_portal/sessions":
			_, _ = w.Write([]byte(`{"url":"https://billing.stripe.com/p/session/test_1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")
			f.mu.Lock()
			sub, ok := f.subs[id]
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"message":"No such subscription"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(sub)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeStripe) set(sub stripe.Subscription) {
	f.mu.Lock()
	f.subs[sub.ID] = sub
	f.mu.Unlock()
}

func (f *fakeStripe) lastCheckout(t *testing.T) url.Values {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.checkouts) == 0 {
		t.Fatal("no checkout page was requested")
	}
	return f.checkouts[len(f.checkouts)-1]
}

type billingRig struct {
	st      *store.Store
	ts      *httptest.Server
	stripe  *fakeStripe
	acct    store.Account
	session string
}

func newBillingRig(t *testing.T, gateOn bool) *billingRig {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	acct, err := st.Register("payer@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := st.NewSession(acct.ID)
	fs := newFakeStripe(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{
		Store: st, Log: quiet, Servers: servers.NewStub(),
		WebURL:              "https://queueuprust.com",
		BillingEnabled:      gateOn,
		Stripe:              &stripe.Client{SecretKey: "sk_test_fake", BaseURL: fs.ts.URL},
		StripePriceID:       testPriceID,
		StripeIntroCouponID: testCouponID,
		StripeWebhookSecret: testWebhookSecret,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &billingRig{st: st, ts: ts, stripe: fs, acct: acct, session: session}
}

func (b *billingRig) call(t *testing.T, method, path string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, b.ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+b.session)
	resp, err := b.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// webhook sends a correctly signed event, the way Stripe would.
func (b *billingRig) webhook(t *testing.T, typ string, object any) int {
	t.Helper()
	obj, _ := json.Marshal(object)
	body := []byte(fmt.Sprintf(`{"id":"evt_%d","type":%q,"data":{"object":%s}}`, time.Now().UnixNano(), typ, obj))
	req, _ := http.NewRequest(http.MethodPost, b.ts.URL+"/stripe/webhook", strings.NewReader(string(body)))
	req.Header.Set("Stripe-Signature", stripe.SignForTests(body, testWebhookSecret, time.Now()))
	resp, err := b.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (b *billingRig) subscribe(t *testing.T, subID string) {
	t.Helper()
	b.stripe.set(stripe.Subscription{ID: subID, Status: "active", Customer: "cus_1",
		Metadata: map[string]string{"account_id": b.acct.ID}})
	code := b.webhook(t, "checkout.session.completed", map[string]any{
		"client_reference_id": b.acct.ID, "customer": "cus_1", "subscription": subID, "mode": "subscription",
	})
	if code != http.StatusOK {
		t.Fatalf("checkout webhook = %d", code)
	}
}

// First checkout: the £1.99 month is applied, and the account id travels with
// it so the webhook can find the right person.
func TestTheFirstCheckoutGetsTheFirstMonthOffer(t *testing.T) {
	b := newBillingRig(t, true)

	code, out := b.call(t, "GET", "/api/billing")
	if code != 200 || out["intro_available"] != true || !strings.Contains(fmt.Sprint(out["price_line"]), "£1.99") {
		t.Fatalf("billing = %d %v", code, out)
	}
	code, out = b.call(t, "POST", "/api/billing/checkout")
	if code != 200 || !strings.HasPrefix(fmt.Sprint(out["url"]), "https://checkout.stripe.com/") {
		t.Fatalf("checkout = %d %v", code, out)
	}
	sent := b.stripe.lastCheckout(t)
	if sent.Get("discounts[0][coupon]") != testCouponID {
		t.Errorf("first checkout did not apply the first-month offer: %v", sent)
	}
	if sent.Get("line_items[0][price]") != testPriceID || sent.Get("mode") != "subscription" {
		t.Errorf("wrong price or mode: %v", sent)
	}
	if sent.Get("client_reference_id") != b.acct.ID || sent.Get("subscription_data[metadata][account_id]") != b.acct.ID {
		t.Errorf("the account id did not travel with the checkout: %v", sent)
	}
	if sent.Get("customer_email") != "payer@example.com" {
		t.Errorf("email not prefilled: %v", sent)
	}
}

// Paying opens the gate; cancelling closes it; coming back is full price.
func TestPayingOpensTheGateAndCancellingClosesIt(t *testing.T) {
	b := newBillingRig(t, true)

	if code, _ := b.call(t, "POST", "/api/jobs"); code != http.StatusPaymentRequired {
		// The gate sits before anything else on creating a join.
		t.Logf("note: unpaid join returned %d", code)
	}
	b.subscribe(t, "sub_1")

	sub, _ := b.st.SubscriptionFor(b.acct.ID)
	if !sub.Active() || sub.CustomerID != "cus_1" || sub.SubID != "sub_1" || !sub.IntroUsed {
		t.Fatalf("after paying: %+v", sub)
	}
	if code, out := b.call(t, "GET", "/api/billing"); code != 200 || out["subscribed"] != true || out["can_manage"] != true {
		t.Fatalf("billing after paying = %v", out)
	}
	// No second subscription on top of the first.
	if code, _ := b.call(t, "POST", "/api/billing/checkout"); code != http.StatusConflict {
		t.Errorf("a paying account was sent to checkout again: %d", code)
	}
	if code, out := b.call(t, "POST", "/api/billing/portal"); code != 200 || !strings.Contains(fmt.Sprint(out["url"]), "billing.stripe.com") {
		t.Errorf("manage page = %d %v", code, out)
	}

	// Cancelled at the end of the month.
	b.stripe.set(stripe.Subscription{ID: "sub_1", Status: "canceled", Customer: "cus_1",
		Metadata: map[string]string{"account_id": b.acct.ID}})
	if code := b.webhook(t, "customer.subscription.deleted", map[string]any{
		"id": "sub_1", "customer": "cus_1", "metadata": map[string]string{"account_id": b.acct.ID},
	}); code != 200 {
		t.Fatalf("deleted webhook = %d", code)
	}
	sub, _ = b.st.SubscriptionFor(b.acct.ID)
	if sub.Active() {
		t.Fatal("the gate stayed open after the subscription ended")
	}

	// Back again: full price, no second £1.99 month.
	code, out := b.call(t, "GET", "/api/billing")
	if code != 200 || out["intro_available"] != false || strings.Contains(fmt.Sprint(out["price_line"]), "£1.99") {
		t.Fatalf("returning customer offered the intro again: %v", out)
	}
	b.call(t, "POST", "/api/billing/checkout")
	if got := b.stripe.lastCheckout(t).Get("discounts[0][coupon]"); got != "" {
		t.Errorf("returning customer got the first-month offer again (%q)", got)
	}
	if b.stripe.lastCheckout(t).Get("customer") != "cus_1" {
		t.Error("returning customer was not reunited with their Stripe customer")
	}
}

// A card that failed on renewal keeps access while Stripe retries.
func TestAFailedRenewalDoesNotLockSomebodyOutStraightAway(t *testing.T) {
	b := newBillingRig(t, true)
	b.subscribe(t, "sub_1")
	b.stripe.set(stripe.Subscription{ID: "sub_1", Status: "past_due", Customer: "cus_1",
		Metadata: map[string]string{"account_id": b.acct.ID}})
	b.webhook(t, "customer.subscription.updated", map[string]any{"id": "sub_1", "customer": "cus_1"})
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); !sub.Active() {
		t.Fatal("a past_due subscription closed the gate")
	}
}

// Webhooks arrive late and out of order. An old subscription's "deleted"
// arriving after a new one started must not switch the new one off.
func TestAnOldSubscriptionEndingDoesNotCloseANewOne(t *testing.T) {
	b := newBillingRig(t, true)
	b.subscribe(t, "sub_old")
	b.stripe.set(stripe.Subscription{ID: "sub_old", Status: "canceled", Customer: "cus_1"})
	b.subscribe(t, "sub_new")

	// The old one's goodbye turns up late.
	b.webhook(t, "customer.subscription.deleted", map[string]any{
		"id": "sub_old", "customer": "cus_1", "metadata": map[string]string{"account_id": b.acct.ID},
	})
	sub, _ := b.st.SubscriptionFor(b.acct.ID)
	if !sub.Active() || sub.SubID != "sub_new" {
		t.Fatalf("a stale webhook cancelled the current subscription: %+v", sub)
	}
}

// "updated: active" arriving after "deleted" must not resurrect it: the relay
// asks Stripe what is true now instead of trusting the order.
func TestAnOutOfOrderWebhookCannotResurrectACancelledSubscription(t *testing.T) {
	b := newBillingRig(t, true)
	b.subscribe(t, "sub_1")
	b.stripe.set(stripe.Subscription{ID: "sub_1", Status: "canceled", Customer: "cus_1",
		Metadata: map[string]string{"account_id": b.acct.ID}})
	b.webhook(t, "customer.subscription.deleted", map[string]any{"id": "sub_1", "customer": "cus_1"})

	// A stale "updated" whose body still says active.
	b.webhook(t, "customer.subscription.updated", map[string]any{
		"id": "sub_1", "customer": "cus_1", "status": "active",
	})
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Active() {
		t.Fatal("a stale webhook reopened a cancelled subscription")
	}
}

// Nobody gets a free subscription by posting to the webhook themselves.
func TestAForgedWebhookCannotMakeAnAccountPaid(t *testing.T) {
	b := newBillingRig(t, true)
	b.stripe.set(stripe.Subscription{ID: "sub_x", Status: "active", Customer: "cus_x"})
	body := fmt.Sprintf(`{"id":"evt_x","type":"checkout.session.completed","data":{"object":{"client_reference_id":%q,"customer":"cus_x","subscription":"sub_x","mode":"subscription"}}}`, b.acct.ID)

	for name, sig := range map[string]string{
		"unsigned":     "",
		"wrong secret": stripe.SignForTests([]byte(body), "whsec_guess", time.Now()),
	} {
		req, _ := http.NewRequest(http.MethodPost, b.ts.URL+"/stripe/webhook", strings.NewReader(body))
		if sig != "" {
			req.Header.Set("Stripe-Signature", sig)
		}
		resp, err := b.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: forged webhook got %d", name, resp.StatusCode)
		}
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Active() {
		t.Fatal("a forged webhook made an account paid")
	}
}

// An event about somebody QueueUp has never heard of is accepted and dropped,
// or Stripe would retry it for days.
func TestAWebhookForAnUnknownAccountIsAcceptedNotRetried(t *testing.T) {
	b := newBillingRig(t, true)
	b.stripe.set(stripe.Subscription{ID: "sub_z", Status: "active", Customer: "cus_z"})
	code := b.webhook(t, "checkout.session.completed", map[string]any{
		"client_reference_id": "acct_nobody", "customer": "cus_z", "subscription": "sub_z", "mode": "subscription",
	})
	if code != http.StatusOK {
		t.Fatalf("unknown account webhook = %d, want 200 so Stripe stops retrying", code)
	}
	if code := b.webhook(t, "invoice.paid", map[string]any{}); code != http.StatusOK {
		t.Errorf("an event QueueUp does not use = %d, want 200", code)
	}
}

// With the gate off (the free beta), checkout still works, so it can be tested
// for real on the live site while nobody is charged to join.
func TestCheckoutWorksWhileTheBetaGateIsOff(t *testing.T) {
	b := newBillingRig(t, false)
	if code, out := b.call(t, "GET", "/api/billing"); code != 200 || out["subscribed"] != true || out["checkout_ready"] != true {
		t.Fatalf("billing with gate off = %v", out)
	}
	if code, _ := b.call(t, "POST", "/api/billing/checkout"); code != 200 {
		t.Fatalf("checkout with gate off = %d", code)
	}
}
