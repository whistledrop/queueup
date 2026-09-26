package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Account is one user. Phase 3 adds real email login; for now an account is
// created from the command line and identified by a token.
type Account struct {
	ID        string
	Email     string
	CreatedAt time.Time
	// EraseAfter is when this account is due to be deleted, if its owner has
	// asked to leave. Zero for almost everybody.
	EraseAfter time.Time
}

// LeavingOn reports whether this account is counting down to deletion.
func (a Account) LeavingOn() (time.Time, bool) {
	return a.EraseAfter, !a.EraseAfter.IsZero()
}

// CreateAccount makes an account and returns its API token. The token is shown
// once and never stored in the clear, so it cannot be recovered later, only
// replaced.
func (s *Store) CreateAccount(email string) (Account, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return Account{}, "", errors.New("an email address is required")
	}
	token, hash := NewToken()
	a := Account{ID: newID("acct"), Email: email, CreatedAt: s.now().UTC()}
	_, err := s.db.Exec(
		`INSERT INTO accounts (id, email, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		a.ID, a.Email, hash, ms(a.CreatedAt))
	if err != nil {
		return Account{}, "", err
	}
	return a, token, nil
}

// accountColumns is every column an Account needs, in the order scanAccount
// reads them. One list, so adding a field to the struct cannot leave one of
// the several lookups behind.
const accountColumns = `id, email, created_at, erase_after`

type scannable interface{ Scan(dest ...any) error }

func scanAccount(row scannable) (Account, error) {
	var a Account
	var created, erase int64
	if err := row.Scan(&a.ID, &a.Email, &created, &erase); err != nil {
		return Account{}, err
	}
	a.CreatedAt = fromMs(created)
	a.EraseAfter = fromMs(erase)
	return a, nil
}

// AccountByToken looks up the account an API token belongs to.
func (s *Store) AccountByToken(token string) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(
		`SELECT `+accountColumns+` FROM accounts WHERE token_hash = ?`, HashToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// AccountByEmail finds an existing account, so the setup script can be run twice
// without creating duplicates.
func (s *Store) AccountByEmail(email string) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(
		`SELECT `+accountColumns+` FROM accounts WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// AccountByID reads one account.
func (s *Store) AccountByID(id string) (Account, error) {
	a, err := scanAccount(s.db.QueryRow(
		`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// DeleteAccount removes an account, but only a clean one: an account that has
// ever paired a PC or run a job keeps its history, and the command refuses
// rather than quietly destroying it. Used to tidy up test accounts.
func (s *Store) DeleteAccount(accountID string) error {
	var devices, jobs int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM devices WHERE account_id = ?`, accountID).Scan(&devices); err != nil {
		return err
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM jobs WHERE account_id = ?`, accountID).Scan(&jobs); err != nil {
		return err
	}
	if devices > 0 || jobs > 0 {
		return fmt.Errorf("that account has %d linked PCs and %d joins on record, so it will not be deleted", devices, jobs)
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE account_id = ?`, accountID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM favourites WHERE account_id = ?`, accountID); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AccountSummary is one row of the customer list: who they are and what they
// have. Enough to answer "who has signed up, did their PC ever connect, and
// have they actually joined anything", which is the whole of early customer
// support.
type AccountSummary struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"created_at"`
	Devices      int       `json:"devices"`
	Jobs         int       `json:"jobs"`
	LastJobAt    time.Time `json:"last_job_at,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at,omitempty"` // newest heartbeat from any of their PCs
	Subscription string    `json:"subscription"`
}

// AllAccounts lists everyone who has signed up, newest first.
//
// Deliberately never returns password hashes, session tokens or API tokens:
// this feeds an admin screen, and a screen has no business holding credentials.
func (s *Store) AllAccounts() ([]AccountSummary, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.email, a.created_at,
		       COALESCE(a.subscription_status, 'none'),
		       (SELECT COUNT(*) FROM devices d WHERE d.account_id = a.id AND d.claimed_at != 0),
		       (SELECT COUNT(*) FROM jobs j WHERE j.account_id = a.id),
		       COALESCE((SELECT MAX(j.created_at) FROM jobs j WHERE j.account_id = a.id), 0),
		       COALESCE((SELECT MAX(d.last_seen_at) FROM devices d WHERE d.account_id = a.id), 0)
		  FROM accounts a
		 ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AccountSummary{}
	for rows.Next() {
		var a AccountSummary
		var created, lastJob, lastSeen int64
		if err := rows.Scan(&a.ID, &a.Email, &created, &a.Subscription,
			&a.Devices, &a.Jobs, &lastJob, &lastSeen); err != nil {
			return nil, err
		}
		a.CreatedAt, a.LastJobAt, a.LastSeenAt = fromMs(created), fromMs(lastJob), fromMs(lastSeen)
		out = append(out, a)
	}
	return out, rows.Err()
}

// GracePeriod is how long somebody has to change their mind after asking to
// delete their account.
//
// Deletion is the one thing here that cannot be undone, and people ask for it
// on bad nights: after a ban, after losing a base, in an argument. Seven days
// is long enough that a bad night has passed and short enough that "delete my
// data" is still an honest answer to give a regulator.
const GracePeriod = 7 * 24 * time.Hour

// RequestErasure starts the countdown, and returns the day it runs out.
//
// Nothing is deleted or switched off yet, and the account keeps working
// exactly as before. That is deliberate: if leaving unlinked their PC today,
// then changing their mind on day five would mean setting it all up again,
// which is not a change of mind, it is a slower deletion.
func (s *Store) RequestErasure(accountID string) (time.Time, error) {
	if _, err := s.AccountByID(accountID); err != nil {
		return time.Time{}, err
	}
	when := s.now().UTC().Add(GracePeriod)
	if _, err := s.db.Exec(`UPDATE accounts SET erase_after = ? WHERE id = ?`,
		ms(when), accountID); err != nil {
		return time.Time{}, err
	}
	return when, nil
}

// CancelErasure is the change of mind. It works right up until the sweep.
func (s *Store) CancelErasure(accountID string) error {
	res, err := s.db.Exec(
		`UPDATE accounts SET erase_after = 0 WHERE id = ? AND erase_after != 0`, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AccountsDueForErasure lists the accounts whose week is up.
func (s *Store) AccountsDueForErasure(now time.Time) ([]Account, error) {
	rows, err := s.db.Query(
		`SELECT `+accountColumns+` FROM accounts WHERE erase_after != 0 AND erase_after <= ?`,
		ms(now.UTC()))
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
