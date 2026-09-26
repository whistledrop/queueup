package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SessionWindow is how long a browser stays signed in without doing anything.
const SessionWindow = 30 * 24 * time.Hour

// ErrBadCredentials is deliberately returned for both "no such email" and
// "wrong password", so the sign-in page cannot be used to find out which email
// addresses have accounts.
var ErrBadCredentials = errors.New("that email or password isn't right")

const authSchema = `
CREATE TABLE IF NOT EXISTS sessions (
  id            TEXT PRIMARY KEY,
  account_id    TEXT NOT NULL REFERENCES accounts(id),
  token_hash    TEXT NOT NULL UNIQUE,
  created_at    INTEGER NOT NULL,
  expires_at    INTEGER NOT NULL,
  last_used_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS sessions_account ON sessions(account_id);
`

// Register creates an account with a password. The password itself is never
// stored: bcrypt keeps a one-way hash, so a stolen database does not hand
// anybody a working password.
func (s *Store) Register(email, password string) (Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(email) < 5 {
		return Account{}, errors.New("that doesn't look like an email address")
	}
	if err := CheckPassword(password); err != nil {
		return Account{}, err
	}
	if _, err := s.AccountByEmail(email); err == nil {
		return Account{}, errors.New("there is already an account with that email address")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, err
	}
	// The API token column is still required by the schema. It is filled with an
	// unusable value so that nothing can sign in with it: passwords and sessions
	// are the only way in now.
	_, unusable := NewToken()
	a := Account{ID: newID("acct"), Email: email, CreatedAt: s.now().UTC()}
	if _, err := s.db.Exec(
		`INSERT INTO accounts (id, email, token_hash, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		a.ID, a.Email, unusable, string(hash), ms(a.CreatedAt)); err != nil {
		return Account{}, err
	}
	return a, nil
}

// SignIn checks an email and password and starts a session.
func (s *Store) SignIn(email, password string) (Account, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var a Account
	var created int64
	var hash sql.NullString
	var erase int64
	err := s.db.QueryRow(
		`SELECT `+accountColumns+`, password_hash FROM accounts WHERE email = ?`, email).
		Scan(&a.ID, &a.Email, &created, &erase, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Spend the time anyway. Answering instantly for unknown addresses and
		// slowly for known ones would leak which is which.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$"+strings.Repeat("x", 53)), []byte(password))
		return Account{}, "", ErrBadCredentials
	}
	if err != nil {
		return Account{}, "", err
	}
	if !hash.Valid || hash.String == "" {
		return Account{}, "", ErrBadCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(password)); err != nil {
		return Account{}, "", ErrBadCredentials
	}
	a.CreatedAt = fromMs(created)
	a.EraseAfter = fromMs(erase)

	token, err := s.NewSession(a.ID)
	if err != nil {
		return Account{}, "", err
	}
	return a, token, nil
}

// NewSession issues a browser session token.
func (s *Store) NewSession(accountID string) (string, error) {
	token, hash := NewToken()
	now := s.now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, account_id, token_hash, created_at, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		newID("sess"), accountID, hash, ms(now), ms(now.Add(SessionWindow)), ms(now))
	if err != nil {
		return "", err
	}
	return token, nil
}

// AccountBySession looks up who is signed in, and refuses expired sessions.
func (s *Store) AccountBySession(token string) (Account, error) {
	var accountID string
	var expires int64
	err := s.db.QueryRow(
		`SELECT account_id, expires_at FROM sessions WHERE token_hash = ?`,
		HashToken(token)).Scan(&accountID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	now := s.now().UTC()
	if now.After(fromMs(expires)) {
		_ = s.SignOut(token)
		return Account{}, ErrNotFound
	}
	if _, err := s.db.Exec(
		`UPDATE sessions SET last_used_at = ? WHERE token_hash = ?`,
		ms(now), HashToken(token)); err != nil {
		return Account{}, err
	}

	a, err := scanAccount(s.db.QueryRow(
		`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, accountID))
	if err != nil {
		return Account{}, fmt.Errorf("loading the account for that session: %w", err)
	}
	return a, nil
}

// SignOut ends one session.
func (s *Store) SignOut(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, HashToken(token))
	return err
}

// tempPasswordAlphabet leaves out look-alike characters, because this password
// is read off one screen and typed into another, usually on a phone.
const tempPasswordAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// SetTemporaryPassword gives an account a new random password and returns it.
//
// This is the beta's answer to "I forgot my password". A proper emailed reset
// needs an email provider, which does not exist yet; until it does, the
// operator can hand somebody a temporary password over whatever channel they
// already talk on. Every existing session is ended, so a password that might
// have been compromised stops working everywhere at once.
func (s *Store) SetTemporaryPassword(accountID string) (string, error) {
	if _, err := s.AccountByID(accountID); err != nil {
		return "", err
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	pw := make([]byte, len(raw))
	for i, v := range raw {
		pw[i] = tempPasswordAlphabet[int(v)%len(tempPasswordAlphabet)]
	}
	password := string(pw)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if _, err := s.db.Exec(`UPDATE accounts SET password_hash = ? WHERE id = ?`,
		string(hash), accountID); err != nil {
		return "", err
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE account_id = ?`, accountID); err != nil {
		return "", err
	}
	return password, nil
}

// ChangePassword sets a new password for somebody who is already signed in and
// knows their current one.
//
// Until this existed, the only way to change a password was to sign out and
// go through "forgotten password", which means anybody who simply wants a
// better password has to prove they can read their own email first. That is
// the wrong shape: forgetting and changing are different problems.
//
// The current password is required. A session left open on a shared PC should
// not be enough to lock the real owner out of their own account.
//
// Every session is then ended, exactly as a reset does, and the caller issues
// a fresh one for the browser doing the changing. Changing a password is how
// somebody throws out a device they no longer trust, and that only works if
// the old sessions actually die.
func (s *Store) ChangePassword(accountID, current, next string) error {
	var hash sql.NullString
	err := s.db.QueryRow(`SELECT password_hash FROM accounts WHERE id = ?`, accountID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !hash.Valid || bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(current)) != nil {
		return ErrBadCredentials
	}
	if err := CheckPassword(next); err != nil {
		return err
	}
	if current == next {
		return errors.New("that is the password you already have")
	}
	fresh, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE accounts SET password_hash = ? WHERE id = ?`,
		string(fresh), accountID); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM sessions WHERE account_id = ?`, accountID)
	return err
}

// CheckOwnPassword reports whether this is really the account holder, for the
// things that are not undoable.
func (s *Store) CheckOwnPassword(accountID, password string) error {
	var hash sql.NullString
	err := s.db.QueryRow(`SELECT password_hash FROM accounts WHERE id = ?`, accountID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !hash.Valid || bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(password)) != nil {
		return ErrBadCredentials
	}
	return nil
}
