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

// AccountByToken looks up the account an API token belongs to.
func (s *Store) AccountByToken(token string) (Account, error) {
	var a Account
	var created int64
	err := s.db.QueryRow(
		`SELECT id, email, created_at FROM accounts WHERE token_hash = ?`,
		HashToken(token)).Scan(&a.ID, &a.Email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	a.CreatedAt = fromMs(created)
	return a, nil
}

// AccountByEmail finds an existing account, so the setup script can be run twice
// without creating duplicates.
func (s *Store) AccountByEmail(email string) (Account, error) {
	var a Account
	var created int64
	err := s.db.QueryRow(
		`SELECT id, email, created_at FROM accounts WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&a.ID, &a.Email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	a.CreatedAt = fromMs(created)
	return a, nil
}

// AccountByID reads one account.
func (s *Store) AccountByID(id string) (Account, error) {
	var a Account
	var created int64
	err := s.db.QueryRow(
		`SELECT id, email, created_at FROM accounts WHERE id = ?`, id).
		Scan(&a.ID, &a.Email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	a.CreatedAt = fromMs(created)
	return a, nil
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
