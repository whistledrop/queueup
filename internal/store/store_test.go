package store_test

import (
	"testing"
	"time"

	"queueup/internal/protocol"
	"queueup/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newAccount(t *testing.T, s *store.Store) (store.Account, string) {
	t.Helper()
	a, token, err := s.CreateAccount("player@example.com")
	if err != nil {
		t.Fatalf("creating account: %v", err)
	}
	return a, token
}

func TestAccountTokenIsNotStoredInTheClear(t *testing.T) {
	s := newStore(t)
	a, token := newAccount(t, s)

	got, err := s.AccountByToken(token)
	if err != nil || got.ID != a.ID {
		t.Fatalf("AccountByToken = %v, %v; want %s", got, err, a.ID)
	}
	if _, err := s.AccountByToken("not-the-token"); err == nil {
		t.Fatal("a wrong token was accepted")
	}
}

// The full pairing handshake: the PC asks for a code with no credentials at all,
// the user claims it from an account, and only then does the PC get a token.
func TestPairingHandshake(t *testing.T) {
	s := newStore(t)
	acct, _ := newAccount(t, s)

	p, err := s.StartPairing("Gaming PC")
	if err != nil {
		t.Fatalf("StartPairing: %v", err)
	}
	if len(p.Code) != 6 {
		t.Fatalf("code %q should be 6 characters", p.Code)
	}

	// Nothing to collect until someone claims it.
	if _, done, err := s.CollectPairingResult(p.ClaimToken); err != nil || done {
		t.Fatalf("CollectPairingResult before claiming = %v, %v; want not done", done, err)
	}

	d, err := s.ClaimPairingCode(acct.ID, p.Code)
	if err != nil {
		t.Fatalf("ClaimPairingCode: %v", err)
	}
	if d.AccountID != acct.ID || !d.Paired() {
		t.Fatalf("device = %+v; want paired to %s", d, acct.ID)
	}

	token, done, err := s.CollectPairingResult(p.ClaimToken)
	if err != nil || !done || token == "" {
		t.Fatalf("CollectPairingResult = %q, %v, %v; want a token", token, done, err)
	}

	// The token authenticates the agent...
	got, err := s.DeviceByToken(token)
	if err != nil || got.ID != d.ID {
		t.Fatalf("DeviceByToken = %v, %v; want %s", got, err, d.ID)
	}
	// ...and can never be collected a second time.
	if _, _, err := s.CollectPairingResult(p.ClaimToken); err == nil {
		t.Fatal("the same pairing token was collected twice")
	}
}

func TestPairingCodeCannotBeUsedTwice(t *testing.T) {
	s := newStore(t)
	acct, _ := newAccount(t, s)
	p, _ := s.StartPairing("PC")

	if _, err := s.ClaimPairingCode(acct.ID, p.Code); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := s.ClaimPairingCode(acct.ID, p.Code); err == nil {
		t.Fatal("the same code was claimed twice")
	}
}

func TestUnknownPairingCodeIsRejectedInPlainLanguage(t *testing.T) {
	s := newStore(t)
	acct, _ := newAccount(t, s)
	_, err := s.ClaimPairingCode(acct.ID, "ZZZZZZ")
	if err == nil {
		t.Fatal("an invented code was accepted")
	}
	if len(err.Error()) < 20 {
		t.Errorf("error %q is too terse to show a user", err)
	}
}

func TestRevokedDeviceStaysRevoked(t *testing.T) {
	s := newStore(t)
	acct, _ := newAccount(t, s)
	p, _ := s.StartPairing("PC")
	d, _ := s.ClaimPairingCode(acct.ID, p.Code)

	if err := s.RevokeDevice(acct.ID, d.ID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	got, _ := s.DeviceByID(d.ID)
	if !got.Revoked() {
		t.Fatal("device is not marked revoked")
	}
	// A second account cannot revoke someone else's PC.
	other, _, _ := s.CreateAccount("someone@example.com")
	if err := s.RevokeDevice(other.ID, d.ID); err == nil {
		t.Fatal("another account was allowed to unlink this PC")
	}
}

func pairedDevice(t *testing.T, s *store.Store) (store.Account, store.Device) {
	t.Helper()
	acct, _ := newAccount(t, s)
	p, err := s.StartPairing("PC")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimPairingCode(acct.ID, p.Code)
	if err != nil {
		t.Fatal(err)
	}
	return acct, d
}

func TestJobLifecycleAndTimeline(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	j, err := s.CreateJob(store.NewJob{
		AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "51.83.128.10:28015",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if j.State != "pending" || !j.Active() {
		t.Fatalf("new job = %+v; want an active pending job", j)
	}

	for _, st := range []protocol.JobStatus{
		{JobID: j.ID, State: "launching", Detail: "Launching Rust.", Attempt: 1},
		{JobID: j.ID, State: "queued", Position: 212, Detail: "In queue, position 212"},
		{JobID: j.ID, State: "queued", Position: 3, Detail: "In queue, position 3"},
		{JobID: j.ID, State: "done", Detail: "You're in."},
	} {
		st.At = time.Now().UTC()
		if _, changed, err := s.ApplyStatus(st); err != nil || !changed {
			t.Fatalf("ApplyStatus(%s) = %v, %v", st.State, changed, err)
		}
	}

	got, err := s.JobByID(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "done" || got.Active() {
		t.Fatalf("job = %+v; want done and inactive", got)
	}

	events, err := s.Events(j.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 { // pending, launching, queued, queued, done
		t.Fatalf("timeline has %d entries, want 5: %+v", len(events), events)
	}
	// "since" is how the phone catches up after a dropped connection.
	tail, err := s.Events(j.ID, events[2].ID)
	if err != nil || len(tail) != 2 {
		t.Fatalf("Events(since third) returned %d entries, want 2", len(tail))
	}
}

// A status arriving after a job has been closed out must not reopen it. This is
// the "cancelled job comes back to life because a dying agent reported in late"
// case.
func TestStatusAfterAJobHasFinishedIsIgnored(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	j, _ := s.CreateJob(store.NewJob{AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015"})

	if err := s.FinishJob(j.ID, "done", "cancelled", "You cancelled the join."); err != nil {
		t.Fatal(err)
	}
	_, changed, err := s.ApplyStatus(protocol.JobStatus{
		JobID: j.ID, State: "queued", Position: 5, At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a late status reopened a finished job")
	}
	got, _ := s.JobByID(j.ID)
	if got.State != "done" {
		t.Fatalf("state = %s, want done", got.State)
	}
}

// This is what makes reboot-resume work: the relay, not the PC, remembers that
// there is a job outstanding.
func TestActiveJobForDeviceIsWhatSurvivesAReboot(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	if _, err := s.ActiveJobForDevice(d.ID); err == nil {
		t.Fatal("found an active job on a device that has none")
	}

	j, _ := s.CreateJob(store.NewJob{AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015"})
	_, _, _ = s.ApplyStatus(protocol.JobStatus{JobID: j.ID, State: "queued", Position: 40, At: time.Now().UTC()})

	got, err := s.ActiveJobForDevice(d.ID)
	if err != nil || got.ID != j.ID || got.Position != 40 {
		t.Fatalf("ActiveJobForDevice = %+v, %v; want job %s at position 40", got, err, j.ID)
	}

	_, _, _ = s.ApplyStatus(protocol.JobStatus{JobID: j.ID, State: "done", At: time.Now().UTC()})
	if _, err := s.ActiveJobForDevice(d.ID); err == nil {
		t.Fatal("a finished job is still being offered for resume")
	}
}
