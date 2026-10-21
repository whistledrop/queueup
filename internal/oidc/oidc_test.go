package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

type rig struct {
	key *rsa.PrivateKey
	p   *Provider
}

func newRig(t *testing.T) *rig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	p := Google([]string{"queueup-web.apps.googleusercontent.com", "queueup-ios.apps.googleusercontent.com"})
	p.KeysURL = srv.URL
	return &rig{key: key, p: p}
}

func (r *rig) sign(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	c, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	d := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func good() map[string]any {
	return map[string]any{
		"iss": "https://accounts.google.com", "aud": "queueup-ios.apps.googleusercontent.com",
		"sub": "10987654321", "email": "Player@Example.com", "email_verified": true,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
	}
}

func TestAGenuineTokenIsAccepted(t *testing.T) {
	r := newRig(t)
	id, err := r.p.Verify(context.Background(), r.sign(t, r.key, "k1", good()), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "10987654321" || id.Email != "player@example.com" || !id.EmailVerified || id.Provider != "google" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestForgedAndWrongTokensAreRefused(t *testing.T) {
	r := newRig(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	with := func(k string, v any) map[string]any { c := good(); c[k] = v; return c }
	cases := map[string]string{
		"signed by somebody else": r.sign(t, other, "k1", good()),
		"for another app":         r.sign(t, r.key, "k1", with("aud", "someone-else.apps.googleusercontent.com")),
		"from another issuer":     r.sign(t, r.key, "k1", with("iss", "https://evil.example.com")),
		"expired":                 r.sign(t, r.key, "k1", with("exp", now.Add(-time.Hour).Unix())),
		"issued in the future":    r.sign(t, r.key, "k1", with("iat", now.Add(time.Hour).Unix())),
		"with no subject":         r.sign(t, r.key, "k1", with("sub", "")),
		"with a key nobody has":   r.sign(t, r.key, "nope", good()),
		"not a token at all":      "hello",
	}
	for name, tok := range cases {
		if _, err := r.p.Verify(context.Background(), tok, "", now); err == nil {
			t.Errorf("a token %s was accepted", name)
		}
	}
}

// Apple's native sign-in puts the SHA-256 of the app's nonce in the token;
// Google's web button puts the nonce itself. Both match; a different one does not.
func TestTheNonceMustMatch(t *testing.T) {
	r := newRig(t)
	h := sha256.Sum256([]byte("abc123"))
	raw := good()
	raw["nonce"] = "abc123"
	hashed := good()
	hashed["nonce"] = hex.EncodeToString(h[:])
	for name, c := range map[string]map[string]any{"raw": raw, "hashed": hashed} {
		tok := r.sign(t, r.key, "k1", c)
		if _, err := r.p.Verify(context.Background(), tok, "abc123", now); err != nil {
			t.Errorf("%s nonce refused: %v", name, err)
		}
		if _, err := r.p.Verify(context.Background(), tok, "different", now); err == nil {
			t.Errorf("%s nonce accepted with the wrong value", name)
		}
	}
}

func TestAnAudienceListIsAccepted(t *testing.T) {
	r := newRig(t)
	c := good()
	c["aud"] = []string{"other", "queueup-web.apps.googleusercontent.com"}
	if _, err := r.p.Verify(context.Background(), r.sign(t, r.key, "k1", c), "", now); err != nil {
		t.Fatal(err)
	}
}

func TestAProviderWithNoAppsSetUpRefusesEverything(t *testing.T) {
	r := newRig(t)
	r.p.Audiences = nil
	if _, err := r.p.Verify(context.Background(), r.sign(t, r.key, "k1", good()), "", now); err == nil {
		t.Fatal("accepted a token with no apps configured")
	}
}
