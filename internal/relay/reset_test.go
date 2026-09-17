package relay

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"queueup/internal/mail"
	"queueup/internal/servers"
	"queueup/internal/store"
)

// fakeResend stands in for Resend, so the reset flow is tested without sending
// anybody an email.
type fakeResend struct {
	mu   sync.Mutex
	sent []map[string]any
	ts   *httptest.Server
}

func newFakeResend(t *testing.T) (*fakeResend, *mail.Sender) {
	t.Helper()
	f := &fakeResend{}
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.sent = append(f.sent, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_1"}`))
	}))
	t.Cleanup(f.ts.Close)
	return f, &mail.Sender{APIKey: "test-key", From: "QueueUp <noreply@queueuprust.com>", BaseURL: f.ts.URL}
}

// emails waits for the expected number of emails, since sending happens in the
// background so the page can answer at once.
func (f *fakeResend) emails(t *testing.T, want int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.sent)
		f.mu.Unlock()
		if n >= want {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.sent...)
}

func resetRig(t *testing.T) (*store.Store, *httptest.Server, *fakeResend) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Register("player@example.com", "the old password"); err != nil {
		t.Fatal(err)
	}
	fake, sender := newFakeResend(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{
		Store: st, Log: quiet, Servers: servers.NewStub(),
		Mail: sender, WebURL: "https://queueuprust.com",
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return st, ts, fake
}

func post(t *testing.T, ts *httptest.Server, path, body string) (int, string) {
	t.Helper()
	resp, err := ts.Client().Post(ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// The whole point: a link arrives, it sets a new password once, and every
// session that existed before is gone.
func TestAResetLinkChangesThePasswordOnceAndSignsEverybodyOut(t *testing.T) {
	st, ts, fake := resetRig(t)
	acct, _ := st.AccountByEmail("player@example.com")
	oldSession, err := st.NewSession(acct.ID)
	if err != nil {
		t.Fatal(err)
	}

	code, _ := post(t, ts, "/api/auth/forgot", `{"email":"Player@Example.com "}`)
	if code != http.StatusOK {
		t.Fatalf("forgot = %d", code)
	}
	sent := fake.emails(t, 1)
	if len(sent) != 1 {
		t.Fatalf("emails sent = %d, want 1", len(sent))
	}
	text, _ := sent[0]["text"].(string)
	i := strings.Index(text, "https://queueuprust.com/reset?token=")
	if i < 0 {
		t.Fatalf("no reset link in the email:\n%s", text)
	}
	token := strings.Fields(text[i+len("https://queueuprust.com/reset?token="):])[0]

	// A password that is too short is refused, and does not burn the link.
	if code, _ := post(t, ts, "/api/auth/reset", `{"token":"`+token+`","password":"short"}`); code != http.StatusBadRequest {
		t.Fatalf("a short password was accepted: %d", code)
	}
	if code, body := post(t, ts, "/api/auth/reset", `{"token":"`+token+`","password":"a brand new password"}`); code != http.StatusOK {
		t.Fatalf("reset = %d %s", code, body)
	}

	if _, _, err := st.SignIn("player@example.com", "the old password"); err == nil {
		t.Error("the old password still works")
	}
	if _, _, err := st.SignIn("player@example.com", "a brand new password"); err != nil {
		t.Errorf("the new password does not work: %v", err)
	}
	if _, err := st.AccountBySession(oldSession); err == nil {
		t.Error("a session from before the reset survived it")
	}
	// Single use.
	if code, _ := post(t, ts, "/api/auth/reset", `{"token":"`+token+`","password":"another password"}`); code != http.StatusBadRequest {
		t.Error("the same link worked twice")
	}
}

// The page must answer identically whether or not the address has an account.
// Otherwise it is a tool for finding out who uses QueueUp.
func TestTheForgottenPasswordPageNeverRevealsWhoHasAnAccount(t *testing.T) {
	_, ts, fake := resetRig(t)

	codeKnown, bodyKnown := post(t, ts, "/api/auth/forgot", `{"email":"player@example.com"}`)
	codeUnknown, bodyUnknown := post(t, ts, "/api/auth/forgot", `{"email":"nobody@example.com"}`)

	if codeKnown != codeUnknown || bodyKnown != bodyUnknown {
		t.Fatalf("answers differ:\n known: %d %s\n unknown: %d %s", codeKnown, bodyKnown, codeUnknown, bodyUnknown)
	}
	if strings.Contains(strings.ToLower(bodyKnown), "player@example.com") {
		t.Error("the answer echoed the address back")
	}
	if sent := fake.emails(t, 1); len(sent) != 1 {
		t.Fatalf("emails sent = %d, want exactly 1 (only the real account)", len(sent))
	}
}

// Nobody gets to use QueueUp to post mail at somebody repeatedly, and the
// limit must not become a way of telling real addresses from made-up ones.
func TestResetRequestsAreLimitedWithoutLeaking(t *testing.T) {
	_, ts, fake := resetRig(t)

	var codes, bodies []string
	for i := 0; i < resetRequestLimit+3; i++ {
		c, b := post(t, ts, "/api/auth/forgot", `{"email":"player@example.com"}`)
		codes = append(codes, http.StatusText(c))
		bodies = append(bodies, b)
	}
	for i := range codes {
		if codes[i] != codes[0] || bodies[i] != bodies[0] {
			t.Fatalf("request %d answered differently once throttled: %s %s", i, codes[i], bodies[i])
		}
	}
	if got := len(fake.emails(t, resetRequestLimit)); got > resetRequestLimit {
		t.Fatalf("sent %d emails, limit was %d", got, resetRequestLimit)
	}
}

// With no email provider configured the page still behaves, and still says
// nothing it should not.
func TestWithoutAnEmailProviderTheResetPageStillAnswersTheSame(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Register("player@example.com", "the old password"); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(New(Config{Store: st, Log: quiet, Servers: servers.NewStub()}))
	defer ts.Close()

	code, body := post(t, ts, "/api/auth/forgot", `{"email":"player@example.com"}`)
	if code != http.StatusOK || !strings.Contains(body, "on its way") {
		t.Fatalf("forgot with no mailer = %d %s", code, body)
	}
	// And no reset was created, so nothing is left dangling.
	if _, _, err := st.StartPasswordReset("player@example.com"); err != nil {
		t.Fatalf("the store should still be usable: %v", err)
	}
}

// An expired link is refused, with the same words as a wrong one.
func TestAnExpiredLinkIsRefused(t *testing.T) {
	st, ts, _ := resetRig(t)
	token, _, err := st.StartPasswordReset("player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ExecForTests(
		`UPDATE password_resets SET expires_at = ? WHERE token_hash = ?`,
		time.Now().Add(-time.Minute).UnixMilli(), store.HashToken(token)); err != nil {
		t.Fatal(err)
	}
	code, expired := post(t, ts, "/api/auth/reset", `{"token":"`+token+`","password":"a brand new password"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("expired link = %d", code)
	}
	_, madeUp := post(t, ts, "/api/auth/reset", `{"token":"not-a-real-token","password":"a brand new password"}`)
	if expired != madeUp {
		t.Errorf("an expired link is distinguishable from a made-up one:\n %s\n %s", expired, madeUp)
	}
}

// The API key must never reach a log or an error message.
func TestTheEmailKeyIsNeverInAnErrorMessage(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// A provider echoing the key back is exactly how the Steam key leaked.
		_, _ = w.Write([]byte(`{"message":"API key re_secret_value is invalid"}`))
	}))
	defer broken.Close()

	sender := &mail.Sender{APIKey: "re_secret_value", From: "a@b.c", BaseURL: broken.URL}
	err := sender.Send(t.Context(), "player@example.com", "subject", "body")
	if err == nil {
		t.Fatal("a refused send reported success")
	}
	if strings.Contains(err.Error(), "re_secret_value") {
		t.Fatalf("the API key is in the error: %v", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("expected the key to be redacted, got: %v", err)
	}
}
