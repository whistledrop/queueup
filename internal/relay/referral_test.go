package relay

import (
	"context"
	"testing"

	"queueup/internal/store"
)

// linkPC gives an account a paired PC, which is half of what makes a referral
// real.
func linkPC(t *testing.T, st *store.Store, accountID string) {
	t.Helper()
	p, err := st.StartPairing("Their PC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimPairingCode(accountID, p.Code); err != nil {
		t.Fatal(err)
	}
}

// mate makes a second account that arrived on someone's code.
func mate(t *testing.T, st *store.Store, email, referrerID string) store.Account {
	t.Helper()
	a, err := st.Register(email, "a good password")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordReferredBy(a.ID, referrerID); err != nil {
		t.Fatal(err)
	}
	return a
}

// The whole point: a month is earned only when the mate has BOTH paid and put
// a PC of their own on the end of it. Either alone is cheap to fake; both at
// once costs a second Windows machine to save three pounds.
func TestAReferralNeedsAPaymentAndAPC(t *testing.T) {
	b := newBillingRig(t, true)
	friend := mate(t, b.st, "friend@example.com", b.acct.ID)

	// Paid, no PC: nothing yet.
	if err := b.st.SetSubscription(friend.ID, "active", "sub_friend"); err != nil {
		t.Fatal(err)
	}
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	if ref, _ := b.st.Referral(b.acct.ID); ref.Earned != 0 {
		t.Fatal("a referral was paid for a payment with no PC behind it")
	}

	// PC as well: now it counts.
	linkPC(t, b.st, friend.ID)
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	ref, _ := b.st.Referral(b.acct.ID)
	if ref.Earned != 1 || ref.Credits != 1 {
		t.Fatalf("referral not earned: %+v", ref)
	}

	// And one mate pays out once, however many times this runs.
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	if ref, _ := b.st.Referral(b.acct.ID); ref.Earned != 1 {
		t.Errorf("one mate paid out %d times", ref.Earned)
	}
}

// A PC with no payment behind it is not a sale either.
func TestAPCWithoutAPaymentEarnsNothing(t *testing.T) {
	b := newBillingRig(t, true)
	friend := mate(t, b.st, "freeloader@example.com", b.acct.ID)
	linkPC(t, b.st, friend.ID)
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	if ref, _ := b.st.Referral(b.acct.ID); ref.Earned != 0 {
		t.Error("a referral was paid for somebody who never paid")
	}
}

// Free access given by hand is a gift, not a sale, and nobody is owed a
// commission on a gift.
func TestACompedMateEarnsNothing(t *testing.T) {
	b := newBillingRig(t, true)
	friend := mate(t, b.st, "comped@example.com", b.acct.ID)
	if err := b.st.SetSubscription(friend.ID, store.StatusComped, ""); err != nil {
		t.Fatal(err)
	}
	linkPC(t, b.st, friend.ID)
	b.srv.maybeAwardReferral(context.Background(), friend.ID)
	if ref, _ := b.st.Referral(b.acct.ID); ref.Earned != 0 {
		t.Error("a referral was paid for a comped account")
	}
}

// Three months, ever. The cap is what puts a floor under how wrong the
// economics can go if somebody finds an angle we have not thought of.
func TestReferralsStopAtThree(t *testing.T) {
	b := newBillingRig(t, true)
	for i, email := range []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com"} {
		f := mate(t, b.st, email, b.acct.ID)
		if err := b.st.SetSubscription(f.ID, "active", "sub_"+email); err != nil {
			t.Fatal(err)
		}
		linkPC(t, b.st, f.ID)
		b.srv.maybeAwardReferral(context.Background(), f.ID)

		ref, _ := b.st.Referral(b.acct.ID)
		want := min(i+1, store.MaxReferralRewards)
		if ref.Earned != want {
			t.Fatalf("after %d mates, earned %d, want %d", i+1, ref.Earned, want)
		}
	}
}

// One PC per account, which the referral reward quietly depends on: if one
// account could collect several PCs, referring yourself would cost nothing
// but a few minutes.
func TestAnAccountCannotLinkASecondPC(t *testing.T) {
	b := newBillingRig(t, true)
	linkPC(t, b.st, b.acct.ID)

	p, err := b.st.StartPairing("Second PC")
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.st.ClaimPairingCode(b.acct.ID, p.Code)
	if err == nil {
		t.Fatal("an account linked a second PC")
	}
	if got := err.Error(); !contains(got, "already has a PC") || !contains(got, "Unlink") {
		t.Errorf("the refusal does not say what to do: %q", got)
	}
}

// Unlinking is how you move PCs, so it has to actually free the slot.
func TestUnlinkingFreesTheSlot(t *testing.T) {
	b := newBillingRig(t, true)
	linkPC(t, b.st, b.acct.ID)
	devices, err := b.st.Devices(b.acct.ID)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices = %v %v", devices, err)
	}
	if err := b.st.RevokeDevice(b.acct.ID, devices[0].ID); err != nil {
		t.Fatal(err)
	}
	linkPC(t, b.st, b.acct.ID) // must not panic or refuse
}

// The code belongs to whoever brought them, and never moves. Unlike a
// marketing channel, a person is owed something, and quietly reassigning the
// debt to whoever shared a link most recently is a way of not paying it.
func TestTheFirstReferrerKeepsThem(t *testing.T) {
	b := newBillingRig(t, true)
	second, err := b.st.Register("second@example.com", "a good password")
	if err != nil {
		t.Fatal(err)
	}
	friend := mate(t, b.st, "loyal@example.com", b.acct.ID)
	if err := b.st.RecordReferredBy(friend.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	ref, _ := b.st.Referral(friend.ID)
	if ref.ReferredBy != b.acct.ID {
		t.Errorf("referrer changed to %q", ref.ReferredBy)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
