package store

import (
	"database/sql"
	"errors"
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
