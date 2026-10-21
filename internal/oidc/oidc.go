// Package oidc checks the ID tokens Google and Apple hand an app when
// somebody taps "Continue with Google" or "Continue with Apple".
//
// The token arrives from the person's own device, so nothing in it is taken
// on trust until three things hold: it is signed by one of the provider's own
// published keys, it was issued by that provider for one of QueueUp's own
// apps, and it has not expired. Only then is the email in it believed.
//
// Plain crypto, no SDK: one signature check and a handful of claims do not
// justify a dependency, and every line here can be read and tested.
package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// Provider is one sign-in provider's rules.
type Provider struct {
	Name    string
	Issuers []string
	KeysURL string
	// Audiences are QueueUp's own client ids with this provider: the iOS
	// app, the Android app, the website. A token for anybody else's app is
	// refused, which is what stops a token from some other site being
	// replayed here.
	Audiences []string

	HTTP *http.Client

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// Google and Apple, with the audiences QueueUp is configured for.
func Google(audiences []string) *Provider {
	return &Provider{
		Name:      "google",
		Issuers:   []string{"https://accounts.google.com", "accounts.google.com"},
		KeysURL:   "https://www.googleapis.com/oauth2/v3/certs",
		Audiences: audiences,
	}
}

func Apple(audiences []string) *Provider {
	return &Provider{
		Name:      "apple",
		Issuers:   []string{"https://appleid.apple.com"},
		KeysURL:   "https://appleid.apple.com/auth/keys",
		Audiences: audiences,
	}
}

// Enabled reports whether this provider has any of QueueUp's apps set up.
func (p *Provider) Enabled() bool { return p != nil && len(p.Audiences) > 0 }

// Identity is who the token says this is, once it has been checked.
type Identity struct {
	Provider string
	// Subject is the provider's permanent id for this person. Emails can
	// change; this does not.
	Subject string
	Email   string
	// EmailVerified is the provider's word that the person controls Email.
	EmailVerified bool
	// PrivateRelay is an Apple "Hide my email" address.
	PrivateRelay bool
}

// ErrInvalid covers every reason a token is refused, so nothing about why is
// handed back to whoever sent it.
var ErrInvalid = errors.New("that sign-in could not be verified")

// Verify checks a token and returns who it is for. nonce, when not empty, must
// match the token's own nonce claim, either as given or as its SHA-256 (Apple's
// native sign-in carries the hash of the nonce the app chose).
func (p *Provider) Verify(ctx context.Context, token, nonce string, now time.Time) (Identity, error) {
	if !p.Enabled() {
		return Identity{}, ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, ErrInvalid
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodePart(parts[0], &header); err != nil || header.Alg != "RS256" || header.Kid == "" {
		return Identity{}, ErrInvalid
	}
	key, err := p.key(ctx, header.Kid, now)
	if err != nil {
		return Identity{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, ErrInvalid
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return Identity{}, ErrInvalid
	}

	var claims struct {
		Iss           string          `json:"iss"`
		Aud           json.RawMessage `json:"aud"`
		Sub           string          `json:"sub"`
		Exp           int64           `json:"exp"`
		Iat           int64           `json:"iat"`
		Email         string          `json:"email"`
		EmailVerified any             `json:"email_verified"`
		IsPrivate     any             `json:"is_private_email"`
		Nonce         string          `json:"nonce"`
	}
	if err := decodePart(parts[1], &claims); err != nil {
		return Identity{}, ErrInvalid
	}
	const skew = 2 * time.Minute
	switch {
	case !slices.Contains(p.Issuers, claims.Iss),
		claims.Sub == "",
		now.After(time.Unix(claims.Exp, 0).Add(skew)),
		claims.Iat > 0 && time.Unix(claims.Iat, 0).After(now.Add(skew)),
		!p.audienceOK(claims.Aud):
		return Identity{}, ErrInvalid
	}
	if nonce != "" {
		hashed := sha256.Sum256([]byte(nonce))
		if claims.Nonce != nonce && claims.Nonce != hex.EncodeToString(hashed[:]) {
			return Identity{}, ErrInvalid
		}
	}
	return Identity{
		Provider:      p.Name,
		Subject:       claims.Sub,
		Email:         strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: truthy(claims.EmailVerified),
		PrivateRelay:  truthy(claims.IsPrivate),
	}, nil
}

// audienceOK accepts "aud" as one string or a list, as the spec allows.
func (p *Provider) audienceOK(raw json.RawMessage) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return slices.Contains(p.Audiences, one)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if slices.Contains(p.Audiences, a) {
				return true
			}
		}
	}
	return false
}

// truthy reads a claim both providers send as either true or "true".
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}

func decodePart(part string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// key returns the provider's signing key with this id, fetching the key set
// at most hourly, and again straight away for an id it has not seen, since
// providers rotate keys.
func (p *Provider) key(ctx context.Context, kid string, now time.Time) (*rsa.PublicKey, error) {
	p.mu.Lock()
	k, ok := p.keys[kid]
	stale := now.Sub(p.fetched) > time.Hour
	recent := now.Sub(p.fetched) < time.Minute
	p.mu.Unlock()
	if ok && !stale {
		return k, nil
	}
	if !ok && recent && p.keys != nil {
		// An unknown key straight after a fetch is a bad token, not a
		// rotation: do not let a stream of them hammer the provider.
		return nil, ErrInvalid
	}
	keys, err := p.fetch(ctx)
	if err != nil {
		if ok {
			return k, nil // keep using what we had while the provider is unreachable
		}
		return nil, fmt.Errorf("fetching %s's signing keys: %w", p.Name, err)
	}
	p.mu.Lock()
	p.keys, p.fetched = keys, now
	p.mu.Unlock()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, ErrInvalid
}

func (p *Provider) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.KeysURL, nil)
	if err != nil {
		return nil, err
	}
	hc := p.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return nil, err
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(out) == 0 {
		return nil, errors.New("no usable keys")
	}
	return out, nil
}
