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
		case r.Method == http.MethodGet && r.URL.Path == "/v1/promotion_codes":
			switch r.URL.Query().Get("code") {
			case "TIKTOK":
				_, _ = w.Write([]byte(`{"data":[{"id":"promo_tiktok","code":"TIKTOK","active":true,
					"coupon":{"amount_off":300,"duration":"once","valid":true}}]}`))
			case "FOREVER":
				_, _ = w.Write([]byte(`{"data":[{"id":"promo_forever","code":"FOREVER","active":true,
					"coupon":{"percent_off":50,"duration":"forever","valid":true}}]}`))
			default:
				_, _ = w.Write([]byte(`{"data":[]}`))
			}
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
		AdminToken:          testAdminToken,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &billingRig{st: st, ts: ts, stripe: fs, acct: acct, session: session}
}

func (b *billingRig) call(t *testing.T, method, path string) (int, map[string]any) {
	t.Helper()
	return b.callBody(t, method, path, "")
}

// admin calls an operator-only endpoint, the way the admin screen does.
func (b *billingRig) admin(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, b.ts.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (b *billingRig) callBody(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, b.ts.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
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

// There is one public price. The lower first month lives behind a code,
// because the code is the only thing that says which channel a customer came
// from, and a discount everybody gets automatically answers nothing.
func TestWithoutACodeThereIsNoDiscount(t *testing.T) {
	b := newBillingRig(t, true)

	code, out := b.call(t, "GET", "/api/billing")
	if code != 200 || !strings.Contains(fmt.Sprint(out["price_line"]), "£4.99") {
		t.Fatalf("billing = %d %v", code, out)
	}
	if strings.Contains(fmt.Sprint(out["price_line"]), "£1.99") {
		t.Error("the discount is being advertised to somebody with no code")
	}

	code, out = b.call(t, "POST", "/api/billing/checkout")
	if code != 200 || !strings.HasPrefix(fmt.Sprint(out["url"]), "https://checkout.stripe.com/") {
		t.Fatalf("checkout = %d %v", code, out)
	}
	sent := b.stripe.lastCheckout(t)
	if sent.Get("discounts[0][promotion_code]") != "" {
		t.Errorf("a discount was applied with no code: %v", sent)
	}
	if sent.Get("subscription_data[metadata][source]") != "" {
		t.Errorf("a source was invented from nowhere: %v", sent)
	}
	// Stripe must never collect a code on its own page: a code we did not
	// resolve is a sale we cannot attribute.
	if sent.Get("allow_promotion_codes") == "true" {
		t.Error("Stripe was left to collect codes, so the channel would be lost")
	}
}

// A code both discounts the first month and says where this customer came
// from, permanently, on the subscription itself.
func TestACodeDiscountsAndAttributes(t *testing.T) {
	b := newBillingRig(t, true)

	code, out := b.callBody(t, "POST", "/api/billing/code", `{"code":"tiktok"}`)
	if code != 200 || out["valid"] != true {
		t.Fatalf("checking a good code = %d %v", code, out)
	}
	if got := fmt.Sprint(out["first_month_pence"]); got != "199" {
		t.Errorf("first month = %s pence, want 199", got)
	}

	if code, out := b.callBody(t, "POST", "/api/billing/checkout", `{"code":"TIKTOK"}`); code != 200 {
		t.Fatalf("checkout with a code = %d %v", code, out)
	}
	sent := b.stripe.lastCheckout(t)
	if sent.Get("discounts[0][promotion_code]") != "promo_tiktok" {
		t.Errorf("the code was not applied: %v", sent)
	}
	if sent.Get("subscription_data[metadata][source]") != "TIKTOK" {
		t.Errorf("the source did not reach the subscription: %v", sent)
	}

	// And it survives the gap between arriving and paying.
	if got, _ := b.st.SourceCode(b.acct.ID); got != "TIKTOK" {
		t.Errorf("source code on the account = %q", got)
	}
}

// Last click wins, so one customer is never owed to two partners.
func TestTheLastCodeWins(t *testing.T) {
	b := newBillingRig(t, true)
	if err := b.st.RememberSourceCode(b.acct.ID, "OLDCODE"); err != nil {
		t.Fatal(err)
	}
	if code, _ := b.callBody(t, "POST", "/api/billing/code", `{"code":"TIKTOK"}`); code != 200 {
		t.Fatal("the newer code was refused")
	}
	if got, _ := b.st.SourceCode(b.acct.ID); got != "TIKTOK" {
		t.Errorf("source code = %q, want the newer one", got)
	}
}

// A code that never expires is a pricing mistake wearing a code's clothes.
func TestAForeverDiscountIsRefused(t *testing.T) {
	b := newBillingRig(t, true)
	_, out := b.callBody(t, "POST", "/api/billing/code", `{"code":"FOREVER"}`)
	if out["valid"] != false {
		t.Errorf("a forever discount was accepted: %v", out)
	}
	if code, _ := b.callBody(t, "POST", "/api/billing/checkout", `{"code":"FOREVER"}`); code != http.StatusBadRequest {
		t.Errorf("checkout accepted a forever discount: %d", code)
	}
}

// A code nobody has heard of is refused clearly rather than silently ignored.
func TestANonsenseCodeIsRefusedKindly(t *testing.T) {
	b := newBillingRig(t, true)
	_, out := b.callBody(t, "POST", "/api/billing/code", `{"code":"NOPE"}`)
	if out["valid"] != false || !strings.Contains(fmt.Sprint(out["line"]), "without one") {
		t.Errorf("unhelpful refusal: %v", out)
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

	// Back again: the standard price, and no discount they did not earn.
	code, out := b.call(t, "GET", "/api/billing")
	if code != 200 || strings.Contains(fmt.Sprint(out["price_line"]), "£1.99") {
		t.Fatalf("returning customer offered a discount: %v", out)
	}
	b.call(t, "POST", "/api/billing/checkout")
	if got := b.stripe.lastCheckout(t).Get("discounts[0][promotion_code]"); got != "" {
		t.Errorf("returning customer got a discount they did not ask for (%q)", got)
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

// Free access given by hand: the friend who tested it, the partner, the
// apology. It must open the gate, must not pretend to be a Stripe
// subscription, and must not be undone by a stray Stripe event.
func TestFreeAccessGivenByHand(t *testing.T) {
	b := newBillingRig(t, true)

	// Locked out to begin with.
	if code, _ := b.call(t, "GET", "/api/billing"); code != 200 {
		t.Fatal("billing unreadable")
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Active() {
		t.Fatal("the gate was open before anything was given")
	}

	code, out := b.admin(t, "POST", "/admin/accounts/"+b.acct.ID+"/comp", `{"free":true}`)
	if code != 200 {
		t.Fatalf("giving free access = %d %v", code, out)
	}
	if !strings.Contains(fmt.Sprint(out["status"]), b.acct.Email) {
		t.Errorf("the confirmation does not name the account: %v", out)
	}

	sub, _ := b.st.SubscriptionFor(b.acct.ID)
	if !sub.Active() || !sub.Comped() {
		t.Fatalf("free access did not open the gate: %+v", sub)
	}

	_, bill := b.call(t, "GET", "/api/billing")
	if bill["subscribed"] != true || bill["comped"] != true {
		t.Errorf("billing does not report free access: %v", bill)
	}
	// They have no Stripe customer, so offering to manage a subscription would
	// send them to a page about nothing.
	if bill["can_manage"] != false {
		t.Error("a comped account was offered the billing portal")
	}
	// And they must never be sent to checkout.
	if code, _ := b.call(t, "POST", "/api/billing/checkout"); code != http.StatusConflict {
		t.Errorf("a comped account was sent to checkout: %d", code)
	}

	// Taking it back puts them on the normal price.
	if code, _ := b.admin(t, "POST", "/admin/accounts/"+b.acct.ID+"/comp", `{"free":false}`); code != 200 {
		t.Fatal("could not take free access back")
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Active() {
		t.Error("the gate stayed open after free access was taken back")
	}
}

// The button must never quietly stop charging somebody who is paying. That
// belongs in Stripe, where the money is.
func TestFreeAccessRefusesToTouchAPayingCustomer(t *testing.T) {
	b := newBillingRig(t, true)
	if err := b.st.SetSubscription(b.acct.ID, "active", "sub_real"); err != nil {
		t.Fatal(err)
	}
	code, out := b.admin(t, "POST", "/admin/accounts/"+b.acct.ID+"/comp", `{"free":true}`)
	if code != http.StatusConflict {
		t.Fatalf("comping a paying customer = %d %v", code, out)
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Comped() {
		t.Error("a paying customer was switched to free access")
	}
}
