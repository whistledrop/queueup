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
	// Status is "none" or "active". Stripe adds nuance later (past_due and so
	// on); anything that is not "active" simply means the gate is closed.
	Status       string
	SubID        string // Stripe's subscription id, once there is one
	SubscribedAt time.Time
}

// Active reports whether the gate is open for this account.
func (s Subscription) Active() bool { return s.Status == "active" }

// SubscriptionFor reads an account's payment state.
func (s *Store) SubscriptionFor(accountID string) (Subscription, error) {
	var sub Subscription
	var at int64
	err := s.db.QueryRow(
		`SELECT subscription_status, subscription_id, subscribed_at FROM accounts WHERE id = ?`,
		accountID).Scan(&sub.Status, &sub.SubID, &at)
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
	if status != "active" && status != "none" {
		return fmt.Errorf("subscription status must be active or none, not %q", status)
	}
	at := int64(0)
	if status == "active" {
		at = ms(s.now().UTC())
	}
	res, err := s.db.Exec(
		`UPDATE accounts SET subscription_status = ?, subscription_id = ?, subscribed_at = ? WHERE id = ?`,
		status, subID, at, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
