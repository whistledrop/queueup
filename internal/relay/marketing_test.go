package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"queueup/internal/store"
)

func init() { marketingPause = 0 }

// marketingRig is the win-back rig, which already has a fake Resend, plus the
// operator's token for the campaign screens.
func newMarketingRig(t *testing.T) *winbackRig {
	t.Helper()
	r := newWinbackRig(t, "", "")
	r.srv.cfg.AdminToken = testAdminToken
	// Pinned, so a test run in a real force wipe week is not handed an extra
	// email it did not expect.
	r.st.SetClockForTest(func() time.Time { return ordinaryDay })
	return r
}

// player makes an account with a linked PC that has had `joins` successful
// joins this month.
func (r *winbackRig) player(t *testing.T, email string, joins int) store.Account {
	t.Helper()
	a, err := r.st.Register(email, "a good password")
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.st.StartPairing("PC")
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.st.ClaimPairingCode(a.ID, p.Code)
	if err != nil {
		t.Fatal(err)
	}
	for range joins {
		j, err := r.st.CreateJob(store.NewJob{AccountID: a.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015"})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.st.FinishJob(j.ID, "done", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Old enough that the first-join email is not also due, unless a test
	// says otherwise.
	if err := r.st.ExecForTests(`UPDATE jobs SET updated_at = ? WHERE account_id = ?`,
		ordinaryDay.Add(-10*24*time.Hour).UnixMilli(), a.ID); err != nil {
		t.Fatal(err)
	}
	return a
}

func (r *winbackRig) to(addr string) []map[string]any {
	var out []map[string]any
	for _, m := range r.sent() {
		if list, ok := m["to"].([]any); ok && len(list) > 0 && list[0] == addr {
			out = append(out, m)
		}
	}
	return out
}

// A day that is not force wipe week, so the wipe email stays out of the way.
var ordinaryDay = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func TestRunningOutOfFreeJoinsSendsOneEmailAMonth(t *testing.T) {
	r := newMarketingRig(t)
	r.player(t, "out@example.com", store.FreeJoinsPerMonth)

	r.srv.sendMarketing(t.Context(), ordinaryDay)
	got := r.to("out@example.com")
	if len(got) != 1 || !strings.Contains(subject(got[0]), "used all 10 free joins") {
		t.Fatalf("emails = %d, first subject %q", len(got), subjectOf(got))
	}
	body := text(got[0])
	if !strings.Contains(body, "https://queueuprust.com/subscribe") {
		t.Errorf("no way to upgrade in:\n%s", body)
	}
	unsubscribeFrom(t, body)

	// Every ten minutes all month, and still only the one.
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	if n := len(r.to("out@example.com")); n != 1 {
		t.Fatalf("emails after a second round = %d, want 1", n)
	}
}

func TestTwoJoinsLeftGetsAHeadsUp(t *testing.T) {
	r := newMarketingRig(t)
	r.player(t, "low@example.com", store.FreeJoinsPerMonth-2)
	r.player(t, "fine@example.com", 3)

	r.srv.sendMarketing(t.Context(), ordinaryDay)
	got := r.to("low@example.com")
	if len(got) != 1 || subject(got[0]) != "2 free joins left this month" {
		t.Fatalf("low emails = %d, subject %q", len(got), subjectOf(got))
	}
	if n := len(r.to("fine@example.com")); n != 0 {
		t.Fatalf("somebody with 7 joins left got %d emails", n)
	}
}

func TestPayingAccountsHearNothingAboutAllowances(t *testing.T) {
	r := newMarketingRig(t)
	a := r.player(t, "paid@example.com", store.FreeJoinsPerMonth+5)
	if err := r.st.SetSubscription(a.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	if n := len(r.to("paid@example.com")); n != 0 {
		t.Fatalf("a paying customer got %d allowance emails", n)
	}
}

func TestSayingStopStopsEverything(t *testing.T) {
	r := newMarketingRig(t)
	a := r.player(t, "stop@example.com", store.FreeJoinsPerMonth)
	if err := r.st.ExecForTests(`UPDATE accounts SET email_optout_at = 1 WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	if n := len(r.to("stop@example.com")); n != 0 {
		t.Fatalf("an unsubscribed account got %d emails", n)
	}
}

// The first join email waits an hour, so it is not on top of the moment, and
// gives up after a week, so it is never stale.
func TestTheFirstJoinEmailComesOnceAndAtTheRightTime(t *testing.T) {
	r := newMarketingRig(t)
	a := r.player(t, "first@example.com", 1)
	setFinished := func(ago time.Duration) {
		if err := r.st.ExecForTests(`UPDATE jobs SET updated_at = ? WHERE account_id = ?`,
			ordinaryDay.Add(-ago).UnixMilli(), a.ID); err != nil {
			t.Fatal(err)
		}
	}

	setFinished(10 * time.Minute)
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	if n := len(r.to("first@example.com")); n != 0 {
		t.Fatalf("sent %d emails ten minutes after the join", n)
	}

	setFinished(2 * time.Hour)
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	got := r.to("first@example.com")
	if len(got) != 1 || !strings.HasPrefix(subject(got[0]), "Your first join worked") {
		t.Fatalf("emails = %d, subject %q", len(got), subjectOf(got))
	}
	if !strings.Contains(text(got[0]), "free wipe join") {
		t.Errorf("the free wipe join was not mentioned:\n%s", text(got[0]))
	}
}

// Somebody who qualifies for two emails on one day gets one.
func TestNobodyHearsFromQueueUpTwiceInADay(t *testing.T) {
	r := newMarketingRig(t)
	a := r.player(t, "busy@example.com", store.FreeJoinsPerMonth)
	// Their first join was two hours ago too.
	if err := r.st.ExecForTests(`UPDATE jobs SET updated_at = ? WHERE account_id = ?`,
		ordinaryDay.Add(-2*time.Hour).UnixMilli(), a.ID); err != nil {
		t.Fatal(err)
	}
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	r.srv.sendMarketing(t.Context(), ordinaryDay)
	if n := len(r.to("busy@example.com")); n != 1 {
		t.Fatalf("emails in one day = %d, want 1", n)
	}
}

func TestForceWipeWeekIsTheMondayToWednesdayBeforeTheFirstThursday(t *testing.T) {
	cases := []struct {
		now  time.Time
		ok   bool
		wipe string
	}{
		// October 2026's first Thursday is the 1st, so its week starts in September.
		{time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC), false, ""},
		{time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC), true, "2026-10-01"},
		{time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC), true, "2026-10-01"},
		{time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), false, ""}, // wipe day itself: too late
		{time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), false, ""},
		{time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC), true, "2026-11-05"},
	}
	for _, c := range cases {
		wipe, ok := wipeWeek(c.now)
		if ok != c.ok || (ok && wipe.Format("2006-01-02") != c.wipe) {
			t.Errorf("%s: got %v %s, want %v %s", c.now, ok, wipe.Format("2006-01-02"), c.ok, c.wipe)
		}
	}
}

// One email, written for where they are: no PC yet, a free wipe join waiting,
// or a paying customer who just needs reminding to schedule.
func TestTheForceWipeEmailFitsThePerson(t *testing.T) {
	r := newMarketingRig(t)
	noPC, err := r.st.Register("nopc@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	_ = noPC
	r.player(t, "free@example.com", 1)
	paid := r.player(t, "paid@example.com", 1)
	if err := r.st.SetSubscription(paid.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}

	wipeMonday := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	r.srv.sendFlows(t.Context(), wipeMonday)

	want := map[string]string{
		"nopc@example.com": "Link your PC",
		"free@example.com": "free wipe join is waiting",
		"paid@example.com": "Schedule your join",
	}
	for addr, phrase := range want {
		got := r.to(addr)
		if len(got) != 1 || !strings.Contains(subject(got[0]), phrase) {
			t.Errorf("%s: emails %d, subject %q, want one containing %q", addr, len(got), subjectOf(got), phrase)
			continue
		}
		if !strings.Contains(subject(got[0]), "Thursday 5 November") {
			t.Errorf("%s: subject %q does not name the day", addr, subject(got[0]))
		}
	}
}

// A campaign goes to its audience a daily allowance at a time, picks up where
// it stopped, and finishes itself.
func TestACampaignSendsItsDailyAllowanceThenFinishes(t *testing.T) {
	r := newMarketingRig(t)
	for _, e := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		r.player(t, e, 0)
	}
	paid := r.player(t, "paying@example.com", 0)
	if err := r.st.SetSubscription(paid.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}

	code, out := r.admin(t, "POST", "/admin/campaigns",
		`{"subject":"QueueUp is free now","body":"Ten joins a month, on us.","segment":"free","daily_limit":2}`)
	if code != http.StatusOK {
		t.Fatalf("create = %d %v", code, out)
	}
	id, _ := out["id"].(string)

	// A draft sends nothing.
	r.srv.sendCampaigns(t.Context(), ordinaryDay)
	if n := len(r.sent()); n != 0 {
		t.Fatalf("a draft sent %d emails", n)
	}

	// The test goes to the address given, marked, and counts for nobody.
	if code, out := r.admin(t, "POST", "/admin/campaigns/"+id+"/test", `{"to":"logan@example.com"}`); code != http.StatusOK {
		t.Fatalf("test send = %d %v", code, out)
	}
	if got := r.to("logan@example.com"); len(got) != 1 || subject(got[0]) != "[Test] QueueUp is free now" {
		t.Fatalf("test email: %d, subject %q", len(got), subjectOf(got))
	}

	if code, out := r.admin(t, "POST", "/admin/campaigns/"+id+"/schedule", `{}`); code != http.StatusOK {
		t.Fatalf("schedule = %d %v", code, out)
	}
	// "Now" to the schedule button is the real clock; this test lives on a fixed day.
	if err := r.st.ExecForTests(`UPDATE campaigns SET start_at = ?`, ordinaryDay.Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// Once scheduled, the words are fixed.
	if code, _ := r.admin(t, "POST", "/admin/campaigns/"+id,
		`{"subject":"Changed","body":"x","segment":"free"}`); code != http.StatusConflict {
		t.Fatalf("editing a scheduled campaign = %d, want 409", code)
	}

	now := ordinaryDay
	r.srv.sendCampaigns(t.Context(), now)
	r.srv.sendCampaigns(t.Context(), now)
	if n := campaignSends(r); n != 2 {
		t.Fatalf("first day sent %d, want the daily limit of 2", n)
	}
	if n := len(r.to("paying@example.com")); n != 0 {
		t.Fatal("a free-plan campaign reached a paying customer")
	}

	// The next day: yesterday's sends are behind the cap and the allowance is fresh.
	if err := r.st.ExecForTests(`UPDATE marketing_sends SET sent_at = ?`, now.Add(-25*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	r.srv.sendCampaigns(t.Context(), now)
	r.srv.sendCampaigns(t.Context(), now)
	if n := campaignSends(r); n != 3 {
		t.Fatalf("after the second day sent %d, want all 3", n)
	}
	c, _ := r.st.CampaignByID(id)
	if c.Status != store.CampaignDone {
		t.Fatalf("status = %s, want done", c.Status)
	}
	for _, m := range r.to("a@example.com") {
		unsubscribeFrom(t, text(m))
	}
}

func TestACampaignWithoutTheAdminTokenIsRefused(t *testing.T) {
	r := newMarketingRig(t)
	req, _ := http.NewRequest("POST", r.ts.URL+"/admin/campaigns",
		strings.NewReader(`{"subject":"x","body":"y","segment":"everyone"}`))
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func campaignSends(r *winbackRig) int {
	n := 0
	for _, m := range r.sent() {
		if subject(m) == "QueueUp is free now" {
			n++
		}
	}
	return n
}

func subjectOf(list []map[string]any) string {
	if len(list) == 0 {
		return ""
	}
	return subject(list[0])
}

// admin calls an operator endpoint with the token.
func (r *winbackRig) admin(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, r.ts.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}
