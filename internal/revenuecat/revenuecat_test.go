package revenuecat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// The shape RevenueCat really returns for somebody on a monthly App Store
// subscription, trimmed to the parts that matter.
const activeAppStore = `{
  "request_date": "2026-10-20T12:00:00Z",
  "subscriber": {
    "original_app_user_id": "acct_123",
    "entitlements": {
      "unlimited": {
        "expires_date": "2026-11-18T09:00:00Z",
        "product_identifier": "queueup_unlimited_monthly",
        "purchase_date": "2026-10-18T09:00:00Z"
      }
    },
    "subscriptions": {
      "queueup_unlimited_monthly": {
        "store": "app_store",
        "expires_date": "2026-11-18T09:00:00Z",
        "period_type": "intro",
        "unsubscribe_detected_at": null,
        "billing_issues_detected_at": null
      }
    }
  }
}`

func TestAnActiveStoreSubscriptionIsRead(t *testing.T) {
	st, err := parseCustomer([]byte(activeAppStore), now)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Store != AppStore || st.Product != "queueup_unlimited_monthly" || st.Ending {
		t.Fatalf("status = %+v", st)
	}
	if !st.ExpiresAt.Equal(time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("expires = %v", st.ExpiresAt)
	}
}

// Turning off auto-renew keeps the month they paid for. It is open, and
// on its way out.
func TestAutoRenewOffIsOpenButEnding(t *testing.T) {
	body := `{"subscriber":{"entitlements":{"unlimited":{"expires_date":"2026-11-01T00:00:00Z","product_identifier":"p"}},
	"subscriptions":{"p":{"store":"play_store","unsubscribe_detected_at":"2026-10-19T00:00:00Z"}}}}`
	st, err := parseCustomer([]byte(body), now)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || !st.Ending || st.Store != PlayStore {
		t.Fatalf("status = %+v, want active, ending, play store", st)
	}
}

// RevenueCat keeps the entitlement in the answer after it lapses, with an
// expiry in the past. That is closed.
func TestAnExpiredEntitlementIsClosed(t *testing.T) {
	body := `{"subscriber":{"entitlements":{"unlimited":{"expires_date":"2026-10-01T00:00:00Z","product_identifier":"p"}},
	"subscriptions":{"p":{"store":"app_store"}}}}`
	st, err := parseCustomer([]byte(body), now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Active {
		t.Fatalf("an expired entitlement reads as active: %+v", st)
	}
}

func TestSomebodyWhoNeverBoughtHasNothing(t *testing.T) {
	st, err := parseCustomer([]byte(`{"subscriber":{"entitlements":{},"subscriptions":{}}}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Active || st.Store != "" {
		t.Fatalf("status = %+v", st)
	}
}

func TestCustomerCallsRevenueCatWithTheSecretKey(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.EscapedPath()
		_, _ = w.Write([]byte(activeAppStore))
	}))
	defer srv.Close()
	c := &Client{SecretKey: "sk_test", BaseURL: srv.URL}
	st, err := c.Customer(context.Background(), "acct_123", now)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk_test" || gotPath != "/v1/subscribers/acct_123" || !st.Active {
		t.Fatalf("auth %q path %q status %+v", gotAuth, gotPath, st)
	}
}

func TestAWebhookWithoutTheSecretIsRefused(t *testing.T) {
	body := []byte(`{"event":{"type":"INITIAL_PURCHASE","app_user_id":"acct_1"}}`)
	if _, err := ReadWebhook(body, "wrong", "right"); err != ErrUnauthorised {
		t.Fatalf("err = %v, want unauthorised", err)
	}
	// An unset secret refuses everything rather than accepting everything.
	if _, err := ReadWebhook(body, "", ""); err != ErrUnauthorised {
		t.Fatalf("err = %v, want unauthorised with no secret configured", err)
	}
	ev, err := ReadWebhook(body, "right", "right")
	if err != nil || ev.Type != "INITIAL_PURCHASE" || len(ev.AccountIDs) != 1 || ev.AccountIDs[0] != "acct_1" {
		t.Fatalf("event = %+v, err %v", ev, err)
	}
}

// A restore on another account moves the purchase. Both accounts change, and
// RevenueCat's anonymous ids are nobody QueueUp knows.
func TestATransferNamesBothAccounts(t *testing.T) {
	body := []byte(`{"event":{"type":"TRANSFER","transferred_from":["acct_old","$RCAnonymousID:abc"],"transferred_to":["acct_new"]}}`)
	ev, err := ReadWebhook(body, "s", "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.AccountIDs) != 2 || ev.AccountIDs[0] != "acct_old" || ev.AccountIDs[1] != "acct_new" {
		t.Fatalf("accounts = %v", ev.AccountIDs)
	}
}
