package relay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"queueup/internal/oidc"
)

const testGoogleClient = "queueup-test.apps.googleusercontent.com"

type googleFake struct {
	key *rsa.PrivateKey
}

func newOAuthRig(t *testing.T) (*winbackRig, *googleFake) {
	t.Helper()
	r := newWinbackRig(t, "", "")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(keys.Close)
	g := oidc.Google([]string{testGoogleClient})
	g.KeysURL = keys.URL
	r.srv.cfg.Google = g
	r.srv.cfg.Apple = oidc.Apple(nil)
	return r, &googleFake{key: key}
}

func (g *googleFake) token(t *testing.T, sub, email string, verified bool) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1"})
	c, _ := json.Marshal(map[string]any{
		"iss": "https://accounts.google.com", "aud": testGoogleClient, "sub": sub,
		"email": email, "email_verified": verified,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	d := sha256.Sum256([]byte(unsigned))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, d[:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func signInWith(t *testing.T, r *winbackRig, provider, token string) (int, map[string]any) {
	t.Helper()
	return r.do(t, "POST", "/api/auth/oauth", "",
		fmt.Sprintf(`{"provider":%q,"id_token":%q}`, provider, token))
}

func TestContinueWithGoogleMakesAnAccountThenSignsBackIn(t *testing.T) {
	r, g := newOAuthRig(t)
	code, out := signInWith(t, r, "google", g.token(t, "g-1", "New@Example.com", true))
	if code != http.StatusCreated || out["session_token"] == nil || out["created"] != true {
		t.Fatalf("first sign-in = %d %v", code, out)
	}
	first := out["account"].(map[string]any)["id"]

	code, out = signInWith(t, r, "google", g.token(t, "g-1", "new@example.com", true))
	if code != http.StatusOK || out["account"].(map[string]any)["id"] != first || out["created"] != false {
		t.Fatalf("second sign-in = %d %v, want the same account back", code, out)
	}

	// It has a way in now, so /me must not send it off to choose a password.
	session, _ := out["session_token"].(string)
	_, me := r.do(t, "GET", "/api/auth/me", session, "")
	if me["has_login"] != true {
		t.Fatalf("/me has_login = %v for a Google account", me["has_login"])
	}
}

// Somebody who made an account on the website with their email can then tap
// Continue with Google and land in that same account: Google vouches for the
// address, which is the proof a password reset relies on too.
func TestAVerifiedGoogleEmailLinksToTheExistingAccount(t *testing.T) {
	r, g := newOAuthRig(t)
	existing, err := r.st.Register("player@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	code, out := signInWith(t, r, "google", g.token(t, "g-2", "player@example.com", true))
	if code != http.StatusOK || out["account"].(map[string]any)["id"] != existing.ID {
		t.Fatalf("sign-in = %d %v, want the existing account", code, out)
	}
}

// An address the provider does not vouch for must never open an account.
func TestAnUnverifiedEmailDoesNotOpenSomebodysAccount(t *testing.T) {
	r, g := newOAuthRig(t)
	if _, err := r.st.Register("victim@example.com", "a good password"); err != nil {
		t.Fatal(err)
	}
	code, out := signInWith(t, r, "google", g.token(t, "attacker", "victim@example.com", false))
	if code != http.StatusConflict || out["session_token"] != nil {
		t.Fatalf("unverified sign-in to an existing address = %d %v, want 409 and no session", code, out)
	}
}

func TestAForgedTokenIsRefused(t *testing.T) {
	r, _ := newOAuthRig(t)
	other := &googleFake{}
	other.key, _ = rsa.GenerateKey(rand.Reader, 2048)
	code, out := signInWith(t, r, "google", other.token(t, "g-3", "x@example.com", true))
	if code != http.StatusUnauthorized || out["session_token"] != nil {
		t.Fatalf("forged token = %d %v", code, out)
	}
}

func TestAProviderThatIsNotSetUpSaysSo(t *testing.T) {
	r, _ := newOAuthRig(t)
	if code, _ := signInWith(t, r, "apple", "whatever"); code != http.StatusNotImplemented {
		t.Fatalf("apple with no client ids = %d, want 501", code)
	}
}

// Typing an address on the landing page only ever opens an account nobody can
// sign in to. A Google account has a way in, so typing its address must not
// hand out a session.
func TestTypingTheEmailOfAGoogleAccountDoesNotOpenIt(t *testing.T) {
	r, g := newOAuthRig(t)
	if code, _ := signInWith(t, r, "google", g.token(t, "g-4", "googler@example.com", true)); code != http.StatusCreated {
		t.Fatal("setup failed")
	}
	code, out := r.do(t, "POST", "/api/auth/start", "", `{"email":"googler@example.com"}`)
	if out["session_token"] != nil || code == http.StatusOK || code == http.StatusCreated {
		t.Fatalf("typing a Google account's email = %d %v, want no session", code, out)
	}
}

// Apple requires in-app deletion, and an Apple-only account has no password
// to type. The signed-in session and the typed word are enough.
func TestAnAccountWithNoPasswordCanStillBeDeleted(t *testing.T) {
	r, g := newOAuthRig(t)
	_, out := signInWith(t, r, "google", g.token(t, "g-5", "leaver@example.com", true))
	session, _ := out["session_token"].(string)
	code, res := r.do(t, "POST", "/api/account/erase", session, `{"confirm":"DELETE"}`)
	if code != http.StatusOK || !strings.Contains(fmt.Sprint(res), "erase_after") {
		t.Fatalf("erase = %d %v", code, res)
	}
}
