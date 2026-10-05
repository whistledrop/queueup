package store

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

// The win-back sequence: three emails to somebody who signed up and has not
// paid, timed against the offer they were shown.
//
// Everything here is decided at the moment of sending, not scheduled ahead.
// There is no queue of future emails to cancel when somebody pays: each
// email asks, as it is about to go, whether this person is still unpaid,
// still subscribed to email, and still has an account. Paying cancels the
// rest of the sequence because the next query simply no longer finds them.
// That is the only cancellation that cannot be forgotten.

// WinbackStage is how far through the sequence an account has got. Stored as
// the number of the last email sent, so 0 is "none yet".
type WinbackStage int

// AccountsDueWinback returns the accounts that should get email number
// `stage` right now.
//
// Each email has a window — from `from` after signup until `until` after
// signup, where `until` is when the NEXT email takes over. Somebody inside
// the window who has not had this email or a later one gets it. Somebody who
// is past the window does not, ever.
//
// That is what stops a pile-up. Without the window, an account that signed
// up thirty hours before this went live would match email one, get it, then
// match email two fifteen minutes later: two marketing emails back to back,
// the first one already stale. With it, they land straight in email two's
// window and get only that. The same rule means the last email cannot go out
// after the offer it is warning about has ended, if the relay happened to be
// down for those hours.
//
// And the reasons never to send marketing at all: paid, or on free access;
// said stop; or asked for their account to be deleted. Plus one that is
// particular to these emails: no promo code. The discounted month exists only
// behind a code, so somebody who arrived without one has no offer for these
// emails to save, warn about or count down to, and telling them otherwise
// would be untrue. Whether the code is still a live one is checked by the
// caller, against Stripe, at the moment of sending.
func (s *Store) AccountsDueWinback(now time.Time, stage WinbackStage, from, until time.Duration) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT `+accountColumns+` FROM accounts
		 WHERE winback_stage < ?
		   AND created_at <= ?
		   AND created_at > ?
		   AND subscription_status NOT IN ('active', ?)
		   AND email_optout_at = 0
		   AND erase_after = 0
		   AND source_code != ''
		 ORDER BY created_at`,
		int(stage), ms(now.Add(-from)), ms(now.Add(-until)), StatusComped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkWinbackSent records that email `stage` has gone to this account, and
// reports whether this call is the one that claimed it.
//
// Called BEFORE the send, like the PC reminder: a crash between the two means
// one email missed, which is better than one sent twice. It only ever moves
// forward, so two sends racing on the same account cannot both claim it, and
// a late email skipped over is never sent afterwards.
func (s *Store) MarkWinbackSent(accountID string, stage WinbackStage) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE accounts SET winback_stage = ? WHERE id = ? AND winback_stage < ?`,
		int(stage), accountID, int(stage))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// UnsubscribeToken returns the token that goes in this account's unsubscribe
// links, making one the first time it is needed.
//
// It is kept in the clear, unlike every other token here, because it has to
// be written into each email and a hash cannot be. That is safe because of
// what it can do, which is exactly one thing: stop marketing email to this
// address. It cannot sign in, cannot read anything, and cannot undo itself.
func (s *Store) UnsubscribeToken(accountID string) (string, error) {
	return s.lazyToken(accountID, "unsubscribe_token")
}

// ContinueToken returns the token that goes in the "pick up where you left
// off" links in the reminder emails, making one the first time it is needed.
//
// It is separate from the unsubscribe token on purpose. That one is handed to
// mail providers in a header and may be fetched by them; this one opens a
// session, and the two must never be able to do each other's job.
//
// What it opens is exactly what typing the address on the landing page opens,
// and no more: the caller applies the same rules, so for an account that has
// paid or has a password it gets nobody in. Kept in the clear for the same
// reason as the unsubscribe token: it has to be written into each email.
func (s *Store) ContinueToken(accountID string) (string, error) {
	return s.lazyToken(accountID, "continue_token")
}

// ErrBadContinue covers every reason a continue link is refused, as one error,
// so the link cannot be used to find out which accounts exist.
var ErrBadContinue = errors.New("that link is not valid")

// AccountByContinueToken finds the account a continue link belongs to.
func (s *Store) AccountByContinueToken(accountID, token string) (Account, error) {
	if accountID == "" || token == "" {
		return Account{}, ErrBadContinue
	}
	var want string
	err := s.db.QueryRow(`SELECT continue_token FROM accounts WHERE id = ?`, accountID).Scan(&want)
	if err != nil || want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
		return Account{}, ErrBadContinue
	}
	return s.AccountByID(accountID)
}

// lazyToken reads a per-account token from `column`, making one the first time.
// Only ever fills an empty one, so two sends racing to make the first cannot
// leave an email in somebody's inbox carrying the loser.
func (s *Store) lazyToken(accountID, column string) (string, error) {
	var tok string
	err := s.db.QueryRow(`SELECT `+column+` FROM accounts WHERE id = ?`, accountID).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if tok != "" {
		return tok, nil
	}
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok = base64.RawURLEncoding.EncodeToString(b)
	if _, err := s.db.Exec(
		`UPDATE accounts SET `+column+` = ? WHERE id = ? AND `+column+` = ''`,
		tok, accountID); err != nil {
		return "", err
	}
	err = s.db.QueryRow(`SELECT `+column+` FROM accounts WHERE id = ?`, accountID).Scan(&tok)
	return tok, err
}

// ErrBadUnsubscribe covers every reason an unsubscribe link is refused. One
// error for all of them, so the page cannot be used to find out which
// account ids exist.
var ErrBadUnsubscribe = errors.New("that unsubscribe link is not valid")

// OptOutOfEmail stops marketing email to an account, given the token from one
// of its links. Doing it twice is fine and still succeeds: somebody pressing
// the button again must not be told it failed.
//
// Permanent. Nothing in QueueUp turns this back off, because an opt-out that
// quietly expires is not an opt-out.
func (s *Store) OptOutOfEmail(accountID, token string) error {
	if accountID == "" || token == "" {
		return ErrBadUnsubscribe
	}
	var want string
	err := s.db.QueryRow(`SELECT unsubscribe_token FROM accounts WHERE id = ?`, accountID).Scan(&want)
	if err != nil || want == "" {
		return ErrBadUnsubscribe
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
		return ErrBadUnsubscribe
	}
	_, err = s.db.Exec(
		`UPDATE accounts SET email_optout_at = ? WHERE id = ? AND email_optout_at = 0`,
		ms(s.now().UTC()), accountID)
	return err
}

// OptedOutOfEmail reports whether this account has said stop.
func (s *Store) OptedOutOfEmail(accountID string) (bool, error) {
	var at int64
	err := s.db.QueryRow(`SELECT email_optout_at FROM accounts WHERE id = ?`, accountID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return at != 0, err
}
