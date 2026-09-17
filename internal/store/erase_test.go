package store_test

import (
	"strings"
	"testing"

	"queueup/internal/store"
)

// "Delete everything you hold about me" has to mean everything, including rows
// in a table an older version created and the current code no longer knows
// about. Production still has the notification subscriptions table from before
// notifications were removed, with rows pointing at accounts.
func TestErasingAnAccountRemovesEverythingAndNothingElse(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	other, err := s.Register("someone-else@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeedback(store.NewFeedback{AccountID: other.ID, Kind: store.FeedbackTyped, Message: "keep me"}); err != nil {
		t.Fatal(err)
	}

	j, err := s.CreateJob(store.NewJob{AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishJob(j.ID, "done", "", "in"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewSession(acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeedback(store.NewFeedback{AccountID: acct.ID, DeviceID: d.ID, Kind: store.FeedbackReport, Body: "SteamID: 7656"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ExecForTests(`CREATE TABLE push_subscriptions (
		id TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id), endpoint TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := s.ExecForTests(`INSERT INTO push_subscriptions VALUES ('p1', ?, 'https://push')`, acct.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.EraseAccount(acct.ID); err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}

	dump, err := s.DebugDump()
	if err != nil {
		t.Fatal(err)
	}
	for _, left := range []string{acct.ID, d.ID, j.ID, acct.Email, "SteamID: 7656", "https://push"} {
		if strings.Contains(dump, left) {
			t.Errorf("%q survived the erase", left)
		}
	}
	if !strings.Contains(dump, "someone-else@example.com") || !strings.Contains(dump, "keep me") {
		t.Error("erasing one account took somebody else's data with it")
	}
	if err := s.EraseAccount(acct.ID); err == nil {
		t.Error("erasing an account that is already gone should say so")
	}
}
