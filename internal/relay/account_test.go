package relay

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"queueup/internal/servers"
	"queueup/internal/store"
)

// testPassword is the one newBetaRig signs its player up with.
const testPassword = "correct horse"

// payingRig is the same player on a relay where the subscription gate is on.
func newPayingRig(t *testing.T) *betaRig {
	t.Helper()
	r := newBetaRig(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{
		Store: r.st, Log: quiet, Servers: servers.NewStub(),
		AdminToken: testAdminToken, BillingEnabled: true,
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	r.ts, r.srv = ts, srv
	return r
}

// Changing a password must end every other session. That is how somebody
// throws a device they no longer trust off their account, and if the old
// sessions survived, the feature would be theatre.
func TestChangingAPasswordEndsEveryOtherSession(t *testing.T) {
	r := newBetaRig(t)

	// A second browser, signed in as the same person.
	other, err := r.st.NewSession(r.acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := r.do(t, "GET", "/api/devices", other, ""); code != http.StatusOK {
		t.Fatalf("the second session did not start out working: %d", code)
	}

	code, body := r.do(t, "POST", "/api/auth/password", r.session,
		`{"current":"`+testPassword+`","next":"a-much-better-one"}`)
	if code != http.StatusOK {
		t.Fatalf("change password = %d %s", code, body)
	}
	var out struct {
		SessionToken string `json:"session_token"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.SessionToken == "" {
		t.Fatal("no replacement session came back, so the browser that changed it is locked out")
	}

	if code, _ := r.do(t, "GET", "/api/devices", other, ""); code != http.StatusUnauthorized {
		t.Errorf("the other session still works: %d", code)
	}
	if code, _ := r.do(t, "GET", "/api/devices", out.SessionToken, ""); code != http.StatusOK {
		t.Errorf("the replacement session does not work: %d", code)
	}
	if _, _, err := r.st.SignIn(r.acct.Email, "a-much-better-one"); err != nil {
		t.Errorf("the new password does not sign in: %v", err)
	}
}

// A session left open on a shared PC must not be enough to take the account.
func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	r := newBetaRig(t)
	code, body := r.do(t, "POST", "/api/auth/password", r.session,
		`{"current":"not-it","next":"a-much-better-one"}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("a wrong current password = %d %s", code, body)
	}
	if _, _, err := r.st.SignIn(r.acct.Email, testPassword); err != nil {
		t.Error("the old password stopped working anyway")
	}
	// And the weak-password rule is the same one signing up uses.
	if code, _ := r.do(t, "POST", "/api/auth/password", r.session,
		`{"current":"`+testPassword+`","next":"short"}`); code != http.StatusBadRequest {
		t.Errorf("a too-short new password = %d", code)
	}
}

// Deleting an account takes the password and the typed word, and then it does
// not delete anything: it starts a week's countdown, because people ask for
// this on bad nights and it is the one action with no undo.
func TestDeletingYourOwnAccountStartsAWeeksCountdown(t *testing.T) {
	r := newBetaRig(t)

	if code, _ := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"wrong","confirm":"DELETE"}`); code != http.StatusUnauthorized {
		t.Error("a wrong password started the countdown")
	}
	if code, _ := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"`+testPassword+`","confirm":""}`); code != http.StatusBadRequest {
		t.Error("the countdown started without the confirmation word")
	}
	if acct, err := r.st.AccountByID(r.acct.ID); err != nil {
		t.Fatal(err)
	} else if _, leaving := acct.LeavingOn(); leaving {
		t.Fatal("two refusals still scheduled the deletion")
	}

	code, body := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"`+testPassword+`","confirm":"DELETE"}`)
	if code != http.StatusOK {
		t.Fatalf("erase = %d %s", code, body)
	}

	// Nothing is gone, and everything still works. Changing their mind on day
	// five has to give them back what they had, not a slower deletion.
	acct, err := r.st.AccountByID(r.acct.ID)
	if err != nil {
		t.Fatal("the account was deleted immediately")
	}
	when, leaving := acct.LeavingOn()
	if !leaving {
		t.Fatal("no countdown was started")
	}
	if d := time.Until(when); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Errorf("the countdown is %v, not about a week", d)
	}
	if code, _ := r.do(t, "GET", "/api/devices", r.session, ""); code != http.StatusOK {
		t.Error("the account stopped working during its own grace period")
	}
	if _, err := r.st.AccountByID(r.acct.ID); err != nil {
		t.Error("the PC link was torn down early")
	}
}

// The change of mind, which is the entire point of the week.
func TestChangingYourMindKeepsTheAccount(t *testing.T) {
	r := newBetaRig(t)
	if code, body := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"`+testPassword+`","confirm":"DELETE"}`); code != http.StatusOK {
		t.Fatalf("erase = %d %s", code, body)
	}
	code, body := r.do(t, "POST", "/api/account/erase/cancel", r.session, "")
	if code != http.StatusOK {
		t.Fatalf("cancel = %d %s", code, body)
	}
	acct, err := r.st.AccountByID(r.acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaving := acct.LeavingOn(); leaving {
		t.Fatal("the countdown is still running")
	}
	// And the sweep leaves them alone.
	r.srv.eraseSweep(time.Now().Add(30 * 24 * time.Hour))
	if _, err := r.st.AccountByID(r.acct.ID); err != nil {
		t.Fatal("the sweep erased an account that had been kept")
	}
	// Pressing it twice is not an error: they have what they asked for.
	if code, _ := r.do(t, "POST", "/api/account/erase/cancel", r.session, ""); code != http.StatusOK {
		t.Error("cancelling twice failed")
	}
}

// When the week is up, the sweep really does erase everything.
func TestTheSweepErasesAccountsWhoseWeekIsUp(t *testing.T) {
	r := newBetaRig(t)
	if code, _ := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"`+testPassword+`","confirm":"DELETE"}`); code != http.StatusOK {
		t.Fatal("could not ask to leave")
	}

	// A day early, nothing happens.
	r.srv.eraseSweep(time.Now().Add(6 * 24 * time.Hour))
	if _, err := r.st.AccountByID(r.acct.ID); err != nil {
		t.Fatal("the account went a day early")
	}

	r.srv.eraseSweep(time.Now().Add(store.GracePeriod + time.Minute))
	if _, err := r.st.AccountByID(r.acct.ID); err == nil {
		t.Error("the account is still there after its week")
	}
	if code, _ := r.do(t, "GET", "/api/devices", r.session, ""); code != http.StatusUnauthorized {
		t.Error("the session outlived the account")
	}
}

// Leaving while a subscription is live would delete everything we know about
// somebody and carry on charging their card, with no account left to cancel
// from. They cancel first.
func TestLeavingIsRefusedWhileTheSubscriptionIsLive(t *testing.T) {
	r := newPayingRig(t)
	if err := r.st.SetSubscription(r.acct.ID, "active", "sub_live"); err != nil {
		t.Fatal(err)
	}
	code, body := r.do(t, "POST", "/api/account/erase", r.session,
		`{"password":"`+testPassword+`","confirm":"DELETE"}`)
	if code != http.StatusConflict {
		t.Fatalf("erase with a live subscription = %d %s", code, body)
	}
	if !strings.Contains(body, "Manage subscription") {
		t.Errorf("the refusal does not say how to fix it: %s", body)
	}
	if _, err := r.st.AccountByID(r.acct.ID); err != nil {
		t.Error("the account was erased anyway")
	}
}

// Nobody signed out can touch either of these.
func TestAccountChangesNeedAnAccount(t *testing.T) {
	r := newBetaRig(t)
	for _, path := range []string{"/api/auth/password", "/api/account/erase"} {
		if code, _ := r.do(t, "POST", path, "", `{}`); code != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d", path, code)
		}
	}
}
