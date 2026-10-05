package relay

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"queueup/internal/servers"
	"queueup/internal/store"
)

type startRig struct {
	st   *store.Store
	ts   *httptest.Server
	mail *fakeResend
}

func newStartRig(t *testing.T) *startRig {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake, sender := newFakeResend(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{Store: st, Log: quiet, Servers: servers.NewStub(), Mail: sender,
		WebURL: "https://queueuprust.com"})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &startRig{st: st, ts: ts, mail: fake}
}

func (r *startRig) post(t *testing.T, path, session, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, r.ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *startRig) start(t *testing.T, email, code string) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "code": code})
	return r.post(t, "/api/auth/start", "", string(b))
}

func (r *startRig) me(t *testing.T, session string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, r.ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+session)
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

// An email address is all it takes to get to the price, and the account it
// makes has no password at all.
func TestAnEmailIsEnoughToReachThePaywall(t *testing.T) {
	r := newStartRig(t)
	code, out := r.start(t, "New@Example.com ", "tiktok")
	if code != http.StatusCreated || out["session_token"] == nil || out["created"] != true {
		t.Fatalf("start = %d %v", code, out)
	}
	session := out["session_token"].(string)
	me := r.me(t, session)
	if me["email"] != "new@example.com" {
		t.Errorf("email stored as %v", me["email"])
	}
	if me["has_password"] != false {
		t.Errorf("has_password = %v, want false", me["has_password"])
	}
	acct, _ := r.st.AccountByEmail("new@example.com")
	if src, _ := r.st.SourceCode(acct.ID); src != "TIKTOK" {
		t.Errorf("arrival code = %q, want TIKTOK", src)
	}
}

// The account with no password cannot be signed into. Not with a guess, and
// not with an empty password either.
func TestNobodyCanSignIntoAnAccountWithNoPassword(t *testing.T) {
	r := newStartRig(t)
	r.start(t, "nopass@example.com", "")
	for _, guess := range []string{"", "        ", "password", "anything at all"} {
		b, _ := json.Marshal(map[string]string{"email": "nopass@example.com", "password": guess})
		if code, out := r.post(t, "/api/auth/login", "", string(b)); code == http.StatusOK || out["session_token"] != nil {
			t.Errorf("signed in with password %q: %d %v", guess, code, out)
		}
	}
}

// What typing an email that already has an account does, by kind of account.
// The rule underneath every case: an email alone never opens an account with
// a password or a payment behind it.
func TestTypingAnExistingEmail(t *testing.T) {
	t.Run("never paid, never had a password: straight back to the paywall", func(t *testing.T) {
		r := newStartRig(t)
		r.start(t, "back@example.com", "TIKTOK")
		code, out := r.start(t, "back@example.com", "YOUTUBE")
		if code != http.StatusOK || out["session_token"] == nil || out["created"] != false {
			t.Fatalf("coming back = %d %v", code, out)
		}
		// Last click wins: the link that brought them back is the one that
		// counts now.
		acct, _ := r.st.AccountByEmail("back@example.com")
		if src, _ := r.st.SourceCode(acct.ID); src != "YOUTUBE" {
			t.Errorf("code = %q, want YOUTUBE", src)
		}
	})

	for _, status := range []string{"active", store.StatusComped} {
		t.Run("no password but "+status+": a link to their inbox, never a session", func(t *testing.T) {
			r := newStartRig(t)
			r.start(t, "paid@example.com", "")
			acct, _ := r.st.AccountByEmail("paid@example.com")
			if err := r.st.SetSubscription(acct.ID, status, "sub_1"); err != nil {
				t.Fatal(err)
			}

			code, out := r.start(t, "paid@example.com", "")
			if code != http.StatusAccepted {
				t.Fatalf("status = %d %v, want 202", code, out)
			}
			if out["session_token"] != nil {
				t.Fatal("an email alone opened an account that has been paid for")
			}
			sent := r.mail.emails(t, 1)
			if len(sent) != 1 {
				t.Fatalf("%d emails, want the one with the link", len(sent))
			}
			if to, _ := sent[0]["to"].([]any); len(to) != 1 || to[0] != "paid@example.com" {
				t.Errorf("link went to %v, not the account's own address", sent[0]["to"])
			}
			if text, _ := sent[0]["text"].(string); !strings.Contains(text, "/reset?token=") {
				t.Errorf("no link to choose a password:\n%s", text)
			}
		})
	}

	t.Run("has a password: sign in, and nothing else", func(t *testing.T) {
		r := newStartRig(t)
		if _, err := r.st.Register("real@example.com", "a good password"); err != nil {
			t.Fatal(err)
		}
		code, out := r.start(t, "real@example.com", "")
		if code != http.StatusConflict || out["sign_in"] != true {
			t.Fatalf("status = %d %v, want 409 telling them to sign in", code, out)
		}
		if out["session_token"] != nil {
			t.Fatal("an email alone opened an account that has a password")
		}
	})
}

// The step after paying: choosing the password. It sets the first one and
// only the first one — it is never a way to change a password without
// knowing the current one.
func TestChoosingThePasswordAfterPaying(t *testing.T) {
	r := newStartRig(t)
	_, out := r.start(t, "finish@example.com", "")
	session := out["session_token"].(string)

	// Too short: the same rule as everywhere else.
	if code, _ := r.post(t, "/api/auth/first-password", session, `{"password":"short"}`); code != http.StatusBadRequest {
		t.Errorf("a short password = %d, want 400", code)
	}

	if code, out := r.post(t, "/api/auth/first-password", session, `{"password":"a good password"}`); code != http.StatusOK {
		t.Fatalf("setting it = %d %v", code, out)
	}
	if me := r.me(t, session); me["has_password"] != true {
		t.Errorf("has_password = %v after setting it", me["has_password"])
	}

	// A second go, from a session left open somewhere, cannot replace it.
	if code, _ := r.post(t, "/api/auth/first-password", session, `{"password":"somebody elses"}`); code != http.StatusConflict {
		t.Errorf("replacing it = %d, want 409", code)
	}

	// And now signing in works, with that password and no other.
	b, _ := json.Marshal(map[string]string{"email": "finish@example.com", "password": "a good password"})
	if code, _ := r.post(t, "/api/auth/login", "", string(b)); code != http.StatusOK {
		t.Errorf("signing in with the new password = %d", code)
	}
	b, _ = json.Marshal(map[string]string{"email": "finish@example.com", "password": "somebody elses"})
	if code, _ := r.post(t, "/api/auth/login", "", string(b)); code == http.StatusOK {
		t.Error("the rejected second password works")
	}

	// From now on, typing the email is "sign in", like any real account.
	if code, out := r.start(t, "finish@example.com", ""); code != http.StatusConflict || out["session_token"] != nil {
		t.Errorf("typing the email after setting a password = %d %v", code, out)
	}

	// Only for somebody signed in.
	if code, _ := r.post(t, "/api/auth/first-password", "", `{"password":"a good password"}`); code != http.StatusUnauthorized {
		t.Errorf("without a session = %d, want 401", code)
	}
}

// The limit on making accounts counts accounts made. Somebody coming back
// to their own must never be told they have made too many.
func TestComingBackIsNotCountedAsANewAccount(t *testing.T) {
	r := newStartRig(t)
	for i := 0; i < 30; i++ {
		if code, out := r.start(t, "loyal@example.com", ""); code != http.StatusCreated && code != http.StatusOK {
			t.Fatalf("visit %d = %d %v", i+1, code, out)
		}
	}
}
