package relay

import (
	"net/http"
	"time"

	"queueup/internal/store"
)

// The free plan, enforced here on the relay.
//
// QueueUp used to be paid-only: the gate on joining was simply "have you got a
// subscription". Now there are two plans, and the gate has to answer a bigger
// question — not "may this person join" but "may this person join THIS, right
// now, given what they have already used this month".
//
// Free gets ten joins a calendar month, plus one scheduled wipe join ever.
// Paid gets everything, without counting.
//
// The web app shows the same numbers, but it is only being polite. This file is
// the law: a request that gets past here is allowed, whatever the browser
// thought.

// planState is everything the product needs to know about what an account may
// do. It is read fresh per request rather than cached: the numbers are cheap
// (one indexed COUNT) and a stale allowance is either a locked-out customer or
// a free join given away twice.
type planState struct {
	// Paid is true for an active subscription, a comped account, or any
	// account at all when billing is switched off entirely.
	Paid bool
	// JoinsUsed is successful, self-started joins this calendar month.
	JoinsUsed int
	// JoinsLeft is what remains of the free allowance. Meaningless when Paid.
	JoinsLeft int
	// ResetsAt is the first instant of next month, when JoinsLeft refills.
	ResetsAt time.Time
	// FreeWipeJoinUsed records that the one free scheduled wipe join is spent.
	FreeWipeJoinUsed bool
}

// planFor reads an account's current standing.
func (s *Server) planFor(acct store.Account) (planState, error) {
	// Billing switched off means everybody is paid, which is how the beta ran
	// and how the test suite runs. Counting joins in that mode would invent a
	// limit nobody agreed to.
	if !s.cfg.BillingEnabled {
		return planState{Paid: true}, nil
	}

	sub, err := s.st.SubscriptionFor(acct.ID)
	if err != nil {
		return planState{}, err
	}
	if sub.Active() {
		return planState{Paid: true}, nil
	}

	now := time.Now().UTC()
	monthStart := store.MonthStart(now)
	used, err := s.st.SuccessfulJoinsSince(acct.ID, monthStart)
	if err != nil {
		return planState{}, err
	}
	wipeUsedAt, err := s.st.FreeWipeJoinUsedAt(acct.ID)
	if err != nil {
		return planState{}, err
	}

	left := store.FreeJoinsPerMonth - used
	if left < 0 {
		left = 0
	}
	return planState{
		JoinsUsed:        used,
		JoinsLeft:        left,
		ResetsAt:         monthStart.AddDate(0, 1, 0),
		FreeWipeJoinUsed: !wipeUsedAt.IsZero(),
	}, nil
}

// Reasons the gate says no. The web app switches on these rather than on the
// wording, so the message can be rewritten without breaking the upgrade prompt
// that goes with it.
const (
	reasonFreeJoinsUsed = "free_joins_used"
	reasonFreeWipeUsed  = "free_wipe_join_used"
)

// writeGateError refuses with 402 and a machine-readable reason, so the app can
// show the right upgrade prompt rather than one generic paywall for everything.
func writeGateError(w http.ResponseWriter, msg, reason string) {
	writeJSON(w, http.StatusPaymentRequired, map[string]string{
		"error":  msg,
		"reason": reason,
	})
}

// allowJoin is the gate for joining now. It returns true when the request may
// proceed, and has already written the refusal when it returns false.
//
// A free player who has run out is refused here, not when the join finishes:
// letting it start and killing it later would waste their time and look like a
// fault rather than a limit.
func (s *Server) allowJoin(w http.ResponseWriter, acct store.Account) bool {
	plan, err := s.planFor(acct)
	if err != nil {
		s.log.Error("reading the plan", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't check your plan. Try again in a moment.")
		return false
	}
	if plan.Paid || plan.JoinsLeft > 0 {
		return true
	}
	writeGateError(w,
		"You've used all 10 free joins this month. They come back on the 1st, or go unlimited for "+
			introLine+", cancel anytime.",
		reasonFreeJoinsUsed)
	return false
}

// allowSchedule is the gate for scheduling a join for later, which is the paid
// feature free players get to try exactly once.
//
// The allowance is spent when the schedule actually fires, not when it is set.
// Somebody who sets one and then changes their mind has not had the thing the
// free wipe join exists to show them, and taking it off them for a cancelled
// plan would be a trap. One pending schedule per PC is already enforced
// elsewhere, so this cannot be used to stack up a queue of free wipe joins.
func (s *Server) allowSchedule(w http.ResponseWriter, acct store.Account) bool {
	plan, err := s.planFor(acct)
	if err != nil {
		s.log.Error("reading the plan", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't check your plan. Try again in a moment.")
		return false
	}
	if plan.Paid || !plan.FreeWipeJoinUsed {
		return true
	}
	writeGateError(w,
		"You've used your free wipe join. Scheduled joins are part of the full plan: "+
			introLine+", cancel anytime.",
		reasonFreeWipeUsed)
	return false
}
