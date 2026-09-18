package stripe

import (
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
