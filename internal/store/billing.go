package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Subscription is what the relay knows about an account's payment state. The
// source of truth will be Stripe; this is the local mirror of it, updated by
// the checkout flow and the webhook when they exist, or by the relay's
// set-subscription command until then.
type Subscription struct {
	// Status is "none", "active" or "comped". Stripe adds nuance later
	// (past_due and so on); anything that is not one of the open states simply
	// means the gate is closed.
	Status       string
	SubID        string // Stripe's subscription id, once there is one
	SubscribedAt time.Time
	// CustomerID is Stripe's customer for this account, set by the first
	// checkout. It is what the "manage subscription" page needs.
	CustomerID string
	// FirstPaidAt is when money first changed hands, kept forever so that a
	// customer who left is not mistaken for one who never arrived.
	FirstPaidAt time.Time
	// IntroUsed records that this account has had the discounted first month.
	// It survives cancelling, which is the point: cancel and resubscribe is
	// full price, or the intro offer is £1.99 forever.
	IntroUsed bool
}

// Active reports whether the gate is open for this account.
func (s Subscription) Active() bool { return s.Status == "active" || s.Status == StatusComped }

// StatusComped is free access given by hand: friends, partners, somebody owed
// an apology. It is deliberately NOT "active", because an account that never
// paid has no Stripe subscription behind it, and calling it active would mean
// the billing portal offering to manage something that does not exist and the
// webhooks free to switch it off.
const StatusComped = "comped"

// Comped reports whether this account was given free access rather than paying.
func (s Subscription) Comped() bool { return s.Status == StatusComped }

// SubscriptionFor reads an account's payment state.
func (s *Store) SubscriptionFor(accountID string) (Subscription, error) {
	var sub Subscription
	var at, intro int64
	var paid int64
	err := s.db.QueryRow(
		`SELECT subscription_status, subscription_id, subscribed_at, stripe_customer_id, intro_used, first_paid_at
		   FROM accounts WHERE id = ?`,
		accountID).Scan(&sub.Status, &sub.SubID, &at, &sub.CustomerID, &intro, &paid)
	sub.IntroUsed = intro != 0
	sub.FirstPaidAt = fromMs(paid)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	sub.SubscribedAt = fromMs(at)
	return sub, nil
}

// SetSubscription records a payment state change. status must be "active" or
// "none".
func (s *Store) SetSubscription(accountID, status, subID string) error {
	if status != "active" && status != "none" && status != StatusComped {
		return fmt.Errorf("subscription status must be active, comped or none, not %q", status)
	}
	at := int64(0)
	if status != "none" {
		at = ms(s.now().UTC())
	}
	res, err := s.db.Exec(
		`UPDATE accounts SET subscription_status = ?, subscription_id = ?, subscribed_at = ? WHERE id = ?`,
		status, subID, at, accountID)
	if err != nil {
		return err
	}
	// subscribed_at is cleared when somebody cancels, which means a customer
	// who paid for six months and left looks exactly like somebody who never
	// paid at all. first_paid_at is written once and never cleared, so churn
	// stays visible: it is the difference between "did not want it" and "did
	// want it, then stopped", and those call for completely different work.
	if status == "active" {
		if _, err := s.db.Exec(
			`UPDATE accounts SET first_paid_at = ? WHERE id = ? AND first_paid_at = 0`,
			ms(s.now().UTC()), accountID); err != nil {
			return err
		}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkStripe records the Stripe customer and subscription behind an account,
// and marks the first-month offer as used: whatever happens to this
// subscription later, the offer has been had.
func (s *Store) LinkStripe(accountID, customerID, subID string) error {
	res, err := s.db.Exec(
		`UPDATE accounts SET stripe_customer_id = ?, subscription_id = ?, intro_used = 1 WHERE id = ?`,
		customerID, subID, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AccountForStripeCustomer finds whose Stripe customer this is.
func (s *Store) AccountForStripeCustomer(customerID string) (string, error) {
	if customerID == "" {
		return "", ErrNotFound
	}
	var id string
	err := s.db.QueryRow(`SELECT id FROM accounts WHERE stripe_customer_id = ?`, customerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
