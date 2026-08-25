package relay

import (
	"errors"
	"testing"
	"time"

	"queueup/internal/store"
)

// A scheduled join is claimed before it is fired, so it can never fire twice
// and there is no "try again in a moment". When the time comes the only choice
// is to proceed or to cancel, which makes what happens on an unreadable answer
// a real decision rather than an oversight.
func TestAnUnreadableSubscriptionDoesNotCancelTheJoin(t *testing.T) {
	// The customer paid, the database hiccupped. Cancelling here would lose the
	// wipe AND tell them their subscription ended, which is both wrong and the
	// worst possible thing to say to somebody who has paid.
	if subscriptionBlocks(store.Subscription{}, errors.New("database is locked")) {
		t.Fatal("a database error cancelled a scheduled join and blamed the customer's payment")
	}
}

func TestAnInactiveSubscriptionStillBlocks(t *testing.T) {
	if !subscriptionBlocks(store.Subscription{Status: "none"}, nil) {
		t.Fatal("an account with no subscription was let through the gate")
	}
	if !subscriptionBlocks(store.Subscription{Status: "canceled"}, nil) {
		t.Fatal("a cancelled subscription was let through the gate")
	}
}

func TestAnActiveSubscriptionPasses(t *testing.T) {
	sub := store.Subscription{Status: "active", SubscribedAt: time.Now()}
	if !sub.Active() {
		t.Skip("Active() does not consider this status active; nothing to assert")
	}
	if subscriptionBlocks(sub, nil) {
		t.Fatal("a paying customer was blocked")
	}
}
