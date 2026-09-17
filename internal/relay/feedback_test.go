package relay

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"queueup/internal/relayclient"
	"queueup/internal/servers"
	"queueup/internal/store"
)

const testAdminToken = "admin-secret"

type betaRig struct {
	st          *store.Store
	ts          *httptest.Server
	acct        store.Account
	session     string
	deviceToken string
	deviceID    string
}

// newBetaRig is a relay with one signed-up player whose PC is linked.
func newBetaRig(t *testing.T) *betaRig {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	acct, err := st.Register("tester@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	session, err := st.NewSession(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.StartPairing("Tester PC")
	if err != nil {
		t.Fatal(err)
	}
	d, err := st.ClaimPairingCode(acct.ID, p.Code)
	if err != nil {
		t.Fatal(err)
	}
	token, done, err := st.CollectPairingResult(p.ClaimToken)
	if err != nil || !done {
		t.Fatalf("pairing did not complete: %v", err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{Store: st, Log: quiet, Servers: servers.NewStub(), AdminToken: testAdminToken})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &betaRig{st: st, ts: ts, acct: acct, session: session, deviceToken: token, deviceID: d.ID}
}

func (r *betaRig) do(t *testing.T, method, path, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, r.ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTypedFeedbackIsFiledUnderTheSignedInAccount(t *testing.T) {
	r := newBetaRig(t)

	if code, _ := r.do(t, "POST", "/api/feedback", "", `{"message":"hi"}`); code != http.StatusUnauthorized {
		t.Fatalf("anonymous feedback was accepted: %d", code)
	}
	if code, body := r.do(t, "POST", "/api/feedback", r.session, `{"message":"   "}`); code != http.StatusBadRequest {
		t.Fatalf("empty feedback: %d %s", code, body)
	}
	code, body := r.do(t, "POST", "/api/feedback", r.session, `{"message":"Joined fine on wipe day!"}`)
	if code != http.StatusCreated {
		t.Fatalf("feedback: %d %s", code, body)
	}

	list, err := r.st.RecentFeedback(10)
	if err != nil || len(list) != 1 {
		t.Fatalf("stored feedback = %v, %v", list, err)
	}
	if list[0].Email != "tester@example.com" || list[0].Kind != store.FeedbackTyped {
		t.Fatalf("filed wrongly: %+v", list[0])
	}
}

// A report sent from the tray must arrive under the account the PC belongs to,
// using nothing but the PC's own token, and go through the real client code.
func TestATrayReportArrivesUnderThePCsAccount(t *testing.T) {
	r := newBetaRig(t)
	report := []byte("QueueUp problem report\n===== AGENT LOG =====\nsomething broke\n")

	err := relayclient.SendReport(context.Background(), r.ts.URL, r.deviceToken, "v9.9.9", report)
	if err != nil {
		t.Fatalf("SendReport: %v", err)
	}

	list, _ := r.st.RecentFeedback(10)
	if len(list) != 1 {
		t.Fatalf("got %d entries, want 1", len(list))
	}
	f := list[0]
	if f.Kind != store.FeedbackReport || f.Email != "tester@example.com" ||
		f.DeviceName != "Tester PC" || f.AgentVersion != "v9.9.9" || f.BodyBytes != len(report) {
		t.Fatalf("report filed wrongly: %+v", f)
	}
	body, err := r.st.FeedbackBody(f.ID)
	if err != nil || body != string(report) {
		t.Fatalf("report body = %q, %v", body, err)
	}
}

func TestReportsNeedARealLinkedPC(t *testing.T) {
	r := newBetaRig(t)
	ctx := context.Background()

	if err := relayclient.SendReport(ctx, r.ts.URL, "not-a-token", "v1", []byte("x")); err == nil {
		t.Fatal("a made-up token was allowed to file a report")
	}
	// A browser session is not a PC.
	if code, _ := r.do(t, "POST", "/agent/report", r.session, `{"report":"x"}`); code != http.StatusUnauthorized {
		t.Fatalf("a browser session filed a PC report: %d", code)
	}
	// An unlinked PC can no longer report.
	if err := r.st.RevokeDevice(r.acct.ID, r.deviceID); err != nil {
		t.Fatal(err)
	}
	if err := relayclient.SendReport(ctx, r.ts.URL, r.deviceToken, "v1", []byte("x")); err == nil {
		t.Fatal("an unlinked PC filed a report")
	}
}

func TestAnOversizedReportIsRefused(t *testing.T) {
	r := newBetaRig(t)
	huge := []byte(strings.Repeat("x", store.MaxReportBody+1))
	if err := relayclient.SendReport(context.Background(), r.ts.URL, r.deviceToken, "v1", huge); err == nil {
		t.Fatal("a report over the limit was accepted")
	}
	if list, _ := r.st.RecentFeedback(10); len(list) != 0 {
		t.Fatalf("an oversized report was stored: %+v", list)
	}
}

func TestAdminFeedbackAndReportBodiesNeedTheAdminToken(t *testing.T) {
	r := newBetaRig(t)
	if err := relayclient.SendReport(context.Background(), r.ts.URL, r.deviceToken, "v1", []byte("the log")); err != nil {
		t.Fatal(err)
	}
	list, _ := r.st.RecentFeedback(10)
	id := list[0].ID

	for _, path := range []string{"/admin/feedback", "/admin/feedback/" + id} {
		for _, tok := range []string{"", r.session, r.deviceToken, "wrong"} {
			if code, _ := r.do(t, "GET", path, tok, ""); code != http.StatusUnauthorized {
				t.Errorf("GET %s with %q = %d, want 401", path, tok, code)
			}
		}
	}

	code, body := r.do(t, "GET", "/admin/feedback", testAdminToken, "")
	if code != http.StatusOK || !strings.Contains(body, "tester@example.com") {
		t.Fatalf("admin list: %d %s", code, body)
	}
	if strings.Contains(body, "the log") {
		t.Fatal("the list carried report bodies; it should only carry sizes")
	}
	code, body = r.do(t, "GET", "/admin/feedback/"+id, testAdminToken, "")
	if code != http.StatusOK || body != "the log" {
		t.Fatalf("admin body: %d %q", code, body)
	}
}

// The beta's stand-in for a password reset: the operator issues a temporary
// password, the old one stops working, and so does every open session.
func TestATemporaryPasswordReplacesTheOldOne(t *testing.T) {
	r := newBetaRig(t)

	if code, _ := r.do(t, "POST", "/admin/accounts/"+r.acct.ID+"/temp-password", r.session, ""); code != http.StatusUnauthorized {
		t.Fatalf("a player issued themselves a password: %d", code)
	}
	code, body := r.do(t, "POST", "/admin/accounts/"+r.acct.ID+"/temp-password", testAdminToken, "")
	if code != http.StatusOK {
		t.Fatalf("temp password: %d %s", code, body)
	}
	var out struct{ Password string }
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Password) < 10 {
		t.Fatalf("bad password response %q", body)
	}

	if _, _, err := r.st.SignIn("tester@example.com", "correct horse"); err == nil {
		t.Fatal("the old password still works")
	}
	if _, _, err := r.st.SignIn("tester@example.com", out.Password); err != nil {
		t.Fatalf("the temporary password does not work: %v", err)
	}
	if code, _ := r.do(t, "GET", "/api/devices", r.session, ""); code != http.StatusUnauthorized {
		t.Fatalf("an old session survived the password change: %d", code)
	}
	if code, _ := r.do(t, "POST", "/admin/accounts/acct_nobody/temp-password", testAdminToken, ""); code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", code)
	}
}

func TestOneConnectionCannotCreateEndlessAccounts(t *testing.T) {
	r := newBetaRig(t)
	for i := 0; i < signUpLimit; i++ {
		body := `{"email":"p` + string(rune('a'+i)) + `@example.com","password":"password123"}`
		if code, resp := r.do(t, "POST", "/api/auth/register", "", body); code != http.StatusCreated {
			t.Fatalf("sign-up %d: %d %s", i, code, resp)
		}
	}
	code, _ := r.do(t, "POST", "/api/auth/register", "", `{"email":"one-more@example.com","password":"password123"}`)
	if code != http.StatusTooManyRequests {
		t.Fatalf("sign-up past the limit = %d, want 429", code)
	}
}

func TestTheAdminViewSaysHowJoinsWent(t *testing.T) {
	r := newBetaRig(t)
	good, _ := r.st.CreateJob(store.NewJob{AccountID: r.acct.ID, DeviceID: r.deviceID, ServerAddr: "1.2.3.4:28015"})
	if err := r.st.FinishJob(good.ID, "done", "", "You're in."); err != nil {
		t.Fatal(err)
	}
	bad, _ := r.st.CreateJob(store.NewJob{AccountID: r.acct.ID, DeviceID: r.deviceID, ServerAddr: "5.6.7.8:28015"})
	if err := r.st.FinishJob(bad.ID, "failed", "steam_problem", "Steam isn't logged in."); err != nil {
		t.Fatal(err)
	}

	code, body := r.do(t, "GET", "/admin/status", testAdminToken, "")
	if code != http.StatusOK {
		t.Fatalf("admin status: %d", code)
	}
	var st struct {
		Outcomes []store.JobOutcome `json:"outcomes_24h"`
		Failures []store.Failure    `json:"failures_7d"`
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, o := range st.Outcomes {
		counts[o.State+"/"+o.Reason] = o.Count
	}
	if counts["done/"] != 1 || counts["failed/steam_problem"] != 1 {
		t.Fatalf("outcomes = %+v", st.Outcomes)
	}
	if len(st.Failures) != 1 || st.Failures[0].Email != "tester@example.com" || st.Failures[0].Reason != "steam_problem" {
		t.Fatalf("failures = %+v", st.Failures)
	}
}
