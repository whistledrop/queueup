package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Referrals.
//
// Each subscriber gets a code of their own. A mate who arrives with it pays
// the lower first month, and once that mate has actually turned up, paid, and
// put a PC of their own on the end of it, the person who sent them gets a
// month at the same lower price.
//
// The reward is deliberately smaller than what it takes to earn it. Somebody
// setting out to game this would need a second Windows PC left switched on and
// a real payment cleared to save three pounds, which is not a trade anybody
// makes. The cap puts a floor under how wrong that calculation can go.

// MaxReferralRewards is how many referral months one account can ever earn.
const MaxReferralRewards = 3

// referralAlphabet leaves out look-alike characters, because these get read
// off one screen and typed into another, or said out loud in a voice channel.
const referralAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewReferralCode invents a code. Six characters from a 32 character alphabet
// is a billion combinations, so codes cannot usefully be guessed at to collect
// somebody else's discount.
func NewReferralCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// A code we cannot make random is worse than no code: fall through to
		// the caller's error handling rather than emit a predictable one.
		return ""
	}
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = referralAlphabet[int(v)%len(referralAlphabet)]
	}
	return string(out)
}

// SetReferralCode records the code minted for this account.
func (s *Store) SetReferralCode(accountID, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return errors.New("a code is required")
	}
	_, err := s.db.Exec(`UPDATE accounts SET referral_code = ? WHERE id = ?`, code, accountID)
	return err
}

// ReferralCode is this account's own code, empty if it has never had one.
func (s *Store) ReferralCode(accountID string) (string, error) {
	var code string
	err := s.db.QueryRow(`SELECT referral_code FROM accounts WHERE id = ?`, accountID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return code, err
}

// AccountForReferralCode finds whose code this is.
func (s *Store) AccountForReferralCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "", ErrNotFound
	}
	var id string
	err := s.db.QueryRow(
		`SELECT id FROM accounts WHERE referral_code = ? AND referral_code != ''`, code).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// RecordReferredBy remembers who sent this account, once.
//
// Only ever set at the beginning and never overwritten: the mate who actually
// brought somebody in keeps them. This is the one place where last click does
// NOT win, because unlike a marketing channel a person is owed something, and
// quietly moving the debt to whoever shared a link most recently would be a
// way of not paying it.
func (s *Store) RecordReferredBy(accountID, referrerID string) error {
	if accountID == "" || referrerID == "" || accountID == referrerID {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE accounts SET referred_by = ? WHERE id = ? AND referred_by = ''`,
		referrerID, accountID)
	return err
}

// Referral is what we know about one account's standing in all this.
type Referral struct {
	Code       string // their own code, to share
	ReferredBy string // who sent them, if anybody
	// Rewarded is true once this account has earned its referrer their month,
	// so one mate can never pay out twice.
	Rewarded bool
	// Credits are months banked and not yet spent; Earned is the lifetime
	// count, which is what the cap is measured against.
	Credits int
	Earned  int
}

func (s *Store) Referral(accountID string) (Referral, error) {
	var r Referral
	var rewarded int
	err := s.db.QueryRow(
		`SELECT referral_code, referred_by, referral_rewarded, referral_credits, referral_earned
		   FROM accounts WHERE id = ?`, accountID).
		Scan(&r.Code, &r.ReferredBy, &rewarded, &r.Credits, &r.Earned)
	if errors.Is(err, sql.ErrNoRows) {
		return Referral{}, ErrNotFound
	}
	r.Rewarded = rewarded != 0
	return r, err
}

// AwardReferral banks one month for the referrer and marks the referee as
// having paid out, in one transaction so a retry cannot pay twice.
//
// It returns false when there was nothing to award: no referrer, already
// rewarded, or the referrer is at the cap.
func (s *Store) AwardReferral(refereeID string) (referrerID string, awarded bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()

	var by string
	var rewarded int
	if err := tx.QueryRow(
		`SELECT referred_by, referral_rewarded FROM accounts WHERE id = ?`, refereeID).
		Scan(&by, &rewarded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	if by == "" || rewarded != 0 {
		return "", false, nil
	}

	var earned int
	if err := tx.QueryRow(`SELECT referral_earned FROM accounts WHERE id = ?`, by).Scan(&earned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}

	// Mark the referee either way. Somebody who arrives after the referrer is
	// already at the cap must not sit there as a debt waiting to be paid the
	// moment a credit is spent.
	if _, err := tx.Exec(`UPDATE accounts SET referral_rewarded = 1 WHERE id = ?`, refereeID); err != nil {
		return "", false, err
	}
	if earned >= MaxReferralRewards {
		return by, false, tx.Commit()
	}
	if _, err := tx.Exec(
		`UPDATE accounts SET referral_credits = referral_credits + 1, referral_earned = referral_earned + 1
		   WHERE id = ?`, by); err != nil {
		return "", false, err
	}
	return by, true, tx.Commit()
}

// SpendReferralCredit takes one banked month off, for the moment it is handed
// to Stripe. It refuses when there is nothing banked, so a double-fire of the
// webhook that applies them cannot give two months away.
func (s *Store) SpendReferralCredit(accountID string) error {
	res, err := s.db.Exec(
		`UPDATE accounts SET referral_credits = referral_credits - 1
		   WHERE id = ? AND referral_credits > 0`, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no referral credit to spend")
	}
	return nil
}

// AccountsWithReferralCredits lists everybody owed a month, for the sweep that
// hands them to Stripe.
func (s *Store) AccountsWithReferralCredits() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT id FROM accounts WHERE referral_credits > 0 AND subscription_id != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
