package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"queueup/internal/relayclient"
	"queueup/internal/store"
)

// Unlinking a PC from the dashboard has to finish the job completely: the PC
// disappears from the account, a join running on it stops, joins scheduled
// for it are cancelled, and its login stops working. Anything left behind
// shows up later as a ghost PC or a wipe join that never fires.
func TestUnlinkingAPCLeavesNothingBehind(t *testing.T) {
	r := newBetaRig(t)

	j, err := r.st.CreateJob(store.NewJob{AccountID: r.acct.ID, DeviceID: r.deviceID, ServerAddr: "1.2.3.4:28015"})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := r.st.CreateSchedule(store.NewSchedule{
		AccountID: r.acct.ID, DeviceID: r.deviceID, ServerAddr: "1.2.3.4:28015",
		FireAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Somebody else cannot unlink it.
	other, _ := r.st.Register("someone-else@example.com", "a good password")
	otherSession, _ := r.st.NewSession(other.ID)
	if code, _ := r.do(t, "POST", "/api/devices/"+r.deviceID+"/revoke", otherSession, ""); code != http.StatusNotFound {
		t.Fatalf("another account unlinked this PC: %d", code)
	}

	if code, body := r.do(t, "POST", "/api/devices/"+r.deviceID+"/revoke", r.session, ""); code != http.StatusOK {
		t.Fatalf("unlink = %d %s", code, body)
	}

	// Gone from the account.
	_, body := r.do(t, "GET", "/api/devices", r.session, "")
	var list struct{ Devices []map[string]any }
	_ = json.Unmarshal([]byte(body), &list)
	if len(list.Devices) != 0 {
		t.Errorf("the unlinked PC is still listed: %v", list.Devices)
	}

	// The running join stopped, reading as cancelled rather than failed.
	got, _ := r.st.JobByID(j.ID)
	if got.State != "done" || got.ReasonCode != "cancelled" {
		t.Errorf("the running join was left as %s/%s", got.State, got.ReasonCode)
	}

	// The scheduled join is cancelled, not left waiting to fail.
	gotSc, _ := r.st.ScheduleByID(sc.ID)
	if gotSc.State != "cancelled" {
		t.Errorf("the scheduled join was left %q", gotSc.State)
	}

	// Its login is dead.
	if err := relayclient.SendReport(context.Background(), r.ts.URL, r.deviceToken, "v1", []byte("x")); err == nil {
		t.Error("the unlinked PC's login still works")
	}

	// Unlinking twice is harmless, and says so.
	if code, _ := r.do(t, "POST", "/api/devices/"+r.deviceID+"/revoke", r.session, ""); code != http.StatusNotFound {
		t.Errorf("second unlink = %d, want 404", code)
	}
}
