package relay

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"queueup/internal/revenuecat"
	"queueup/internal/servers"
	"queueup/internal/store"
	"queueup/internal/stripe"
)

const testRCWebhookSecret = "rc-webhook-secret"

// fakeRevenueCat answers GET /v1/subscribers/{id} with whatever the test last
// said that customer has.
type fakeRevenueCat struct {
	mu        sync.Mutex
	customers map[string]string
	ts        *httptest.Server
}

func newFakeRevenueCat(t *testing.T) *fakeRevenueCat {
	f := &fakeRevenueCat{customers: map[string]string{}}
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk_rc_fake" {
			http.Error(w, `{"message":"bad key"}`, http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/subscribers/")
		f.mu.Lock()
		body, ok := f.customers[id]
		f.mu.Unlock()
		if !ok {
			body = `{"subscriber":{"entitlements":{},"subscriptions":{}}}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.ts.Close)
	return f
}

// set gives a customer Unlimited from a store, expiring at expires, with
// auto-renew on or off.
func (f *fakeRevenueCat) set(id, store string, expires time.Time, autoRenewOff bool) {
	unsub := "null"
	if autoRenewOff {
		unsub = `"2026-10-20T00:00:00Z"`
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customers[id] = fmt.Sprintf(`{"subscriber":{
	  "entitlements":{"unlimited":{"expires_date":%q,"product_identifier":"queueup_unlimited_monthly"}},
	  "subscriptions":{"queueup_unlimited_monthly":{"store":%q,"expires_date":%q,"unsubscribe_detected_at":%s}}}}`,
		expires.UTC().Format(time.RFC3339), store, expires.UTC().Format(time.RFC3339), unsub)
}

type storeRig struct {
	*billingRig
	rc *fakeRevenueCat
}

func newStoreRig(t *testing.T) *storeRig {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	acct, err := st.Register("phone@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := st.NewSession(acct.ID)
	fs := newFakeStripe(t)
	rc := newFakeRevenueCat(t)
	srv := New(Config{
		Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Servers: servers.NewStub(),
		WebURL:                  "https://queueuprust.com",
		BillingEnabled:          true,
		Stripe:                  &stripe.Client{SecretKey: "sk_test_fake", BaseURL: fs.ts.URL},
		StripePriceID:           testPriceID,
		StripeIntroCouponID:     testCouponID,
		StripeWebhookSecret:     testWebhookSecret,
		RevenueCat:              &revenuecat.Client{SecretKey: "sk_rc_fake", BaseURL: rc.ts.URL},
		RevenueCatWebhookSecret: testRCWebhookSecret,
		AdminToken:              testAdminToken,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &storeRig{
		billingRig: &billingRig{st: st, ts: ts, srv: srv, stripe: fs, acct: acct, session: session},
		rc:         rc,
	}
}

func (b *storeRig) rcWebhook(t *testing.T, auth, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, b.ts.URL+"/revenuecat/webhook", strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	resp, err := b.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (b *storeRig) event(t *testing.T, typ string) int {
	t.Helper()
	return b.rcWebhook(t, testRCWebhookSecret,
		fmt.Sprintf(`{"api_version":"1.0","event":{"type":%q,"app_user_id":%q}}`, typ, b.acct.ID))
}

// Buying in the app opens Unlimited the moment the app says so, without
// waiting for RevenueCat's webhook, and the free plan's limits stop applying.
func TestBuyingInTheAppOpensUnlimitedAtOnce(t *testing.T) {
	b := newStoreRig(t)
	if _, out := b.call(t, "GET", "/api/billing"); out["plan"] != "free" {
		t.Fatalf("before buying, plan = %v", out["plan"])
	}
	b.rc.set(b.acct.ID, "app_store", time.Now().Add(30*24*time.Hour), false)

	code, out := b.call(t, "POST", "/api/billing/store-sync")
	if code != http.StatusOK {
		t.Fatalf("store sync = %d %v", code, out)
	}
	if out["plan"] != "paid" || out["paying"] != true || out["managed_by"] != "app_store" {
		t.Fatalf("after buying: plan %v paying %v managed_by %v", out["plan"], out["paying"], out["managed_by"])
	}
	// Managed in the App Store, so the website's Stripe manage page has
	// nothing to offer.
	if out["can_manage"] != false {
		t.Fatalf("can_manage = %v for a store subscription", out["can_manage"])
	}
}

// Turning off auto-renew in the phone's settings keeps the month they paid
// for, and says when it ends; the expiry then closes it.
func TestCancellingInTheStoreKeepsTheMonthThenEnds(t *testing.T) {
	b := newStoreRig(t)
	end := time.Now().Add(10 * 24 * time.Hour)
	b.rc.set(b.acct.ID, "play_store", end, false)
	if code := b.event(t, "INITIAL_PURCHASE"); code != http.StatusOK {
		t.Fatalf("purchase webhook = %d", code)
	}

	b.rc.set(b.acct.ID, "play_store", end, true)
	if code := b.event(t, "CANCELLATION"); code != http.StatusOK {
		t.Fatalf("cancellation webhook = %d", code)
	}
	sub, _ := b.st.SubscriptionFor(b.acct.ID)
	if !sub.Active() || sub.EndsAt.IsZero() {
		t.Fatalf("after cancelling: active %v, ends %v; want still open with an end date", sub.Active(), sub.EndsAt)
	}

	b.rc.set(b.acct.ID, "play_store", time.Now().Add(-time.Minute), true)
	if code := b.event(t, "EXPIRATION"); code != http.StatusOK {
		t.Fatalf("expiry webhook = %d", code)
	}
	if _, out := b.call(t, "GET", "/api/billing"); out["plan"] != "free" || out["paying"] != false {
		t.Fatalf("after expiry: plan %v paying %v", out["plan"], out["paying"])
	}
}

// Somebody already paying on the website must not be switched off because
// their App Store subscription, or the lack of one, says otherwise.
func TestTheStoreNeverClosesAWebsiteSubscription(t *testing.T) {
	b := newStoreRig(t)
	b.subscribe(t, "sub_web")

	// RevenueCat knows nothing about them: an EXPIRATION for this account
	// (from an old trial, say) must not touch the Stripe subscription.
	if code := b.event(t, "EXPIRATION"); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	_, out := b.call(t, "GET", "/api/billing")
	if out["paying"] != true || out["managed_by"] != "stripe" {
		t.Fatalf("website subscriber after a store expiry: paying %v managed_by %v", out["paying"], out["managed_by"])
	}

	// And a store purchase on top leaves the website one in charge.
	b.rc.set(b.acct.ID, "app_store", time.Now().Add(30*24*time.Hour), false)
	if code := b.event(t, "INITIAL_PURCHASE"); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.SubID != "sub_web" {
		t.Fatalf("subscription id = %q, want the website's to stay", sub.SubID)
	}
}

// The other way round: Stripe tidying up an old website subscription must
// not close the one bought in the app.
func TestAnOldWebsiteSubscriptionEndingLeavesTheStoreOneOpen(t *testing.T) {
	b := newStoreRig(t)
	b.subscribe(t, "sub_old")
	b.stripe.set(stripe.Subscription{ID: "sub_old", Status: "canceled", Customer: "cus_1",
		Metadata: map[string]string{"account_id": b.acct.ID}})
	b.webhook(t, "customer.subscription.deleted", map[string]any{"id": "sub_old", "customer": "cus_1"})

	b.rc.set(b.acct.ID, "app_store", time.Now().Add(30*24*time.Hour), false)
	b.call(t, "POST", "/api/billing/store-sync")

	// A late Stripe event about the old one arrives after the store purchase.
	b.webhook(t, "customer.subscription.updated", map[string]any{"id": "sub_old", "customer": "cus_1"})
	if _, out := b.call(t, "GET", "/api/billing"); out["paying"] != true || out["managed_by"] != "app_store" {
		t.Fatalf("after a late Stripe event: paying %v managed_by %v", out["paying"], out["managed_by"])
	}
}

// Paying twice for one thing ends in a chargeback, so the website will not
// sell to somebody already subscribed in the app.
func TestTheWebsiteWillNotSellToAnAppSubscriber(t *testing.T) {
	b := newStoreRig(t)
	b.rc.set(b.acct.ID, "app_store", time.Now().Add(30*24*time.Hour), false)
	b.call(t, "POST", "/api/billing/store-sync")
	if code, _ := b.call(t, "POST", "/api/billing/checkout"); code != http.StatusConflict {
		t.Fatalf("checkout for an app subscriber = %d, want 409", code)
	}
}

func TestARevenueCatWebhookWithoutTheSecretChangesNothing(t *testing.T) {
	b := newStoreRig(t)
	b.rc.set(b.acct.ID, "app_store", time.Now().Add(30*24*time.Hour), false)
	body := fmt.Sprintf(`{"event":{"type":"INITIAL_PURCHASE","app_user_id":%q}}`, b.acct.ID)
	if code := b.rcWebhook(t, "guess", body); code != http.StatusUnauthorized {
		t.Fatalf("forged webhook = %d, want 401", code)
	}
	if sub, _ := b.st.SubscriptionFor(b.acct.ID); sub.Active() {
		t.Fatal("a forged webhook opened the gate")
	}
}

// RevenueCat's dashboard test button, and anybody QueueUp has never heard of,
// are accepted rather than retried for days.
func TestARevenueCatEventForNobodyIsAccepted(t *testing.T) {
	b := newStoreRig(t)
	if code := b.rcWebhook(t, testRCWebhookSecret, `{"event":{"type":"TEST","app_user_id":"nobody"}}`); code != http.StatusOK {
		t.Fatalf("test event = %d, want 200", code)
	}
}
