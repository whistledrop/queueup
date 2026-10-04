package relay

import (
	"net/http"
	"strings"
	"testing"

	"queueup/internal/store"
)

// Somebody types their email on the landing page and never comes back. That
// person was invisible before: interested enough to click a link from a video
// and type an address, and then something stopped them.
func TestAnAddressIsKeptEvenWhenTheySignUpNoFurther(t *testing.T) {
	r := newBetaRig(t)

	if code, body := r.do(t, "POST", "/api/leads", "", `{"email":"maybe@example.com","code":"TIKTOK"}`); code != http.StatusOK {
		t.Fatalf("recording a lead = %d %s", code, body)
	}
	people, err := r.st.Funnel()
	if err != nil {
		t.Fatal(err)
	}
	var found *store.Person
	for i := range people {
		if people[i].Email == "maybe@example.com" {
			found = &people[i]
		}
	}
	if found == nil {
		t.Fatal("the address was not kept")
	}
	if found.Stage != store.StageLead {
		t.Errorf("stage = %q, want lead", found.Stage)
	}
	if found.SourceCode != "TIKTOK" {
		t.Errorf("source = %q, want the code they arrived on", found.SourceCode)
	}
}

// And when they come back and finish, they stop being a lead rather than
// being counted twice.
func TestFinishingSignupStopsThemBeingALead(t *testing.T) {
	r := newBetaRig(t)
	if code, _ := r.do(t, "POST", "/api/leads", "", `{"email":"finisher@example.com"}`); code != http.StatusOK {
		t.Fatal("could not record the lead")
	}
	if code, body := r.do(t, "POST", "/api/auth/register", "",
		`{"email":"finisher@example.com","password":"a good password"}`); code != http.StatusCreated {
		t.Fatalf("register = %d %s", code, body)
	}

	people, _ := r.st.Funnel()
	seen := 0
	for _, p := range people {
		if p.Email == "finisher@example.com" {
			seen++
			if p.Stage != store.StageSignedUp {
				t.Errorf("stage = %q, want signed_up", p.Stage)
			}
		}
	}
	if seen != 1 {
		t.Errorf("appears %d times in the funnel, want once", seen)
	}
}

// The difference that matters: somebody who paid and left is not the same as
// somebody who never paid. One is a product problem and the other is a pitch
// problem, and they need opposite work.
func TestSomebodyWhoPaidAndLeftIsNotMistakenForSomebodyWhoNeverPaid(t *testing.T) {
	r := newBetaRig(t)
	never, err := r.st.Register("never@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	_ = never

	if err := r.st.SetSubscription(r.acct.ID, "active", "sub_1"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.SetSubscription(r.acct.ID, "none", ""); err != nil {
		t.Fatal(err)
	}

	stages := map[string]store.Stage{}
	people, _ := r.st.Funnel()
	for _, p := range people {
		stages[p.Email] = p.Stage
	}
	if stages[r.acct.Email] != store.StageLapsed {
		t.Errorf("the one who paid and left reads as %q, want lapsed", stages[r.acct.Email])
	}
	if stages["never@example.com"] != store.StageSignedUp {
		t.Errorf("the one who never paid reads as %q, want signed_up", stages["never@example.com"])
	}
}

// An address we hold and cannot get out is one we do not really have.
func TestTheListCanBeTakenOutAsACSV(t *testing.T) {
	r := newBetaRig(t)
	if code, _ := r.do(t, "POST", "/api/leads", "", `{"email":"export@example.com","code":"YOUTUBE"}`); code != http.StatusOK {
		t.Fatal("could not record the lead")
	}
	code, body := r.do(t, "GET", "/admin/funnel.csv", testAdminToken, "")
	if code != http.StatusOK {
		t.Fatalf("csv = %d %s", code, body)
	}
	for _, want := range []string{"email,stage,source_code", "export@example.com", "YOUTUBE", r.acct.Email} {
		if !strings.Contains(body, want) {
			t.Errorf("the export is missing %q", want)
		}
	}
	if code, _ := r.do(t, "GET", "/admin/funnel.csv", "", ""); code != http.StatusUnauthorized {
		t.Error("the whole customer list is downloadable without the admin token")
	}
}

// Rubbish in the box is refused, and the endpoint never says who already has
// an account: it is open to anybody, so answering differently for a known
// address would be a way of finding out who is a customer here.
func TestTheOpenEndpointGivesNothingAway(t *testing.T) {
	r := newBetaRig(t)
	if code, _ := r.do(t, "POST", "/api/leads", "", `{"email":"nonsense"}`); code != http.StatusBadRequest {
		t.Error("a non-address was accepted")
	}
	code, body := r.do(t, "POST", "/api/leads", "", `{"email":"`+r.acct.Email+`"}`)
	if code != http.StatusOK {
		t.Fatalf("an existing address = %d", code)
	}
	if strings.Contains(strings.ToLower(body), "already") || strings.Contains(strings.ToLower(body), "exists") {
		t.Errorf("the answer reveals that this address has an account: %s", body)
	}
}
