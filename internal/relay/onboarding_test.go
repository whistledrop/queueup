package relay

import (
	"context"
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

func onboardingRig(t *testing.T) (*Server, *store.Store, *httptest.Server, *fakeResend) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake, sender := newFakeResend(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{Store: st, Log: quiet, Servers: servers.NewStub(), Mail: sender, WebURL: "https://queueuprust.com"})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, st, ts, fake
}

// On the phone, one tap puts the PC link in their inbox, and it cannot be
// used to flood that inbox.
func TestEmailingYourselfThePCLink(t *testing.T) {
	_, st, ts, fake := onboardingRig(t)
	acct, _ := st.Register("phone-first@example.com", "a good password")
	session, _ := st.NewSession(acct.ID)

	press := func() int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/onboarding/pc-link", nil)
		req.Header.Set("Authorization", "Bearer "+session)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := press(); code != http.StatusOK {
		t.Fatalf("first press = %d", code)
	}
	sent := fake.emails(t, 1)
	if len(sent) != 1 {
		t.Fatalf("emails = %d", len(sent))
	}
	to, _ := sent[0]["to"].([]any)
	if len(to) != 1 || to[0] != "phone-first@example.com" {
		t.Errorf("sent to %v", sent[0]["to"])
	}
	if text, _ := sent[0]["text"].(string); !strings.Contains(text, "https://queueuprust.com/get") {
		t.Errorf("no PC link in the email:\n%s", text)
	}

	for i := 1; i < pcLinkLimit; i++ {
		press()
	}
	if code := press(); code != http.StatusTooManyRequests {
		t.Errorf("press past the limit = %d, want 429", code)
	}

	// Only for somebody signed in.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/onboarding/pc-link", nil)
	resp, _ := ts.Client().Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous press = %d", resp.StatusCode)
	}
}

// The reminder: once, only to people with no PC, only in their first week.
func TestTheSetUpYourPCReminderGoesOnceToTheRightPeople(t *testing.T) {
	srv, st, _, fake := onboardingRig(t)
	now := time.Now()

	aged := func(email string, age time.Duration) store.Account {
		a, err := st.Register(email, "a good password")
		if err != nil {
			t.Fatal(err)
		}
		if err := st.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`, now.Add(-age).UnixMilli(), a.ID); err != nil {
			t.Fatal(err)
		}
		return a
	}
	due := aged("forgot-their-pc@example.com", 26*time.Hour)
	aged("just-signed-up@example.com", 2*time.Hour)            // too soon
	aged("long-gone@example.com", 10*24*time.Hour)             // too late
	linked := aged("already-linked@example.com", 26*time.Hour) // has a PC
	p, _ := st.StartPairing("PC")
	if _, err := st.ClaimPairingCode(linked.ID, p.Code); err != nil {
		t.Fatal(err)
	}

	srv.sendPCReminders(context.Background(), now)
	srv.sendPCReminders(context.Background(), now.Add(time.Hour)) // and again later

	sent := fake.emails(t, 1)
	if len(sent) != 1 {
		var to []any
		for _, e := range sent {
			to = append(to, e["to"])
		}
		t.Fatalf("reminders sent to %v, want exactly one, to %s", to, due.Email)
	}
	if to, _ := sent[0]["to"].([]any); len(to) != 1 || to[0] != due.Email {
		t.Errorf("reminder went to %v", sent[0]["to"])
	}
	if text, _ := sent[0]["text"].(string); !strings.Contains(text, "only reminder") {
		t.Errorf("the reminder does not promise it is the only one:\n%s", text)
	}
}

// With email down, nobody is marked as reminded, so they still get their one
// reminder once email is back.
func TestNoEmailMeansNobodyLosesTheirReminder(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{Store: st, Log: quiet, Servers: servers.NewStub()}) // no mail
	a, _ := st.Register("waiting@example.com", "a good password")
	_ = st.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`, time.Now().Add(-26*time.Hour).UnixMilli(), a.ID)

	srv.sendPCReminders(context.Background(), time.Now())
	still, err := st.AccountsAwaitingPC(time.Now(), pcReminderAfter, pcReminderCutoff)
	if err != nil || len(still) != 1 {
		t.Fatalf("after a run with no email, awaiting = %v, %v; want the account still waiting", still, err)
	}
}

// With the gate on, the "your PC isn't linked yet" reminder goes only to
// people who have actually paid.
//
// Everything that email asks for is behind the paywall: it points at the
// download page and talks them through linking a PC they cannot join from.
// Sending it to somebody who has not paid tells them to do the wrong thing,
// and spends the single message we allow ourselves doing it.
func TestTheReminderDoesNotGoToPeopleWhoHaveNotPaid(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake, sender := newFakeResend(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{
		Store: st, Log: quiet, Servers: servers.NewStub(), Mail: sender,
		WebURL: "https://queueuprust.com", BillingEnabled: true,
	})

	now := time.Now()
	aged := func(email string) store.Account {
		a, err := st.Register(email, "a good password")
		if err != nil {
			t.Fatal(err)
		}
		if err := st.ExecForTests(`UPDATE accounts SET created_at = ? WHERE id = ?`,
			now.Add(-26*time.Hour).UnixMilli(), a.ID); err != nil {
			t.Fatal(err)
		}
		return a
	}
	paid := aged("paid-but-no-pc@example.com")
	if err := st.SetSubscription(paid.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}
	comped := aged("free-access@example.com")
	if err := st.SetSubscription(comped.ID, store.StatusComped, ""); err != nil {
		t.Fatal(err)
	}
	unpaid := aged("never-paid@example.com")

	srv.sendPCReminders(context.Background(), now)

	sent := fake.emails(t, 2)
	got := map[string]bool{}
	for _, e := range sent {
		if to, _ := e["to"].([]any); len(to) == 1 {
			got[to[0].(string)] = true
		}
	}
	if !got[paid.Email] {
		t.Error("a paying customer with no PC was not reminded")
	}
	if !got[comped.Email] {
		t.Error("somebody on free access was not reminded; they can use QueueUp")
	}
	if got[unpaid.Email] {
		t.Error("somebody who never paid was told to go and set up a PC")
	}

	// And they keep their reminder. Paying on day three must still bring it,
	// because it has not been spent.
	if err := st.SetSubscription(unpaid.ID, "active", "sub_2"); err != nil {
		t.Fatal(err)
	}
	srv.sendPCReminders(context.Background(), now.Add(time.Hour))
	after := fake.emails(t, 3)
	var reminded bool
	for _, e := range after {
		if to, _ := e["to"].([]any); len(to) == 1 && to[0] == unpaid.Email {
			reminded = true
		}
	}
	if !reminded {
		t.Error("somebody who paid later never got the reminder they had not used")
	}
}
