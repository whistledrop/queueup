package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Password resets, the emailed kind.
//
// The operator can already hand somebody a temporary password, which is what
// the beta started with. That does not scale past a handful of people and it
// means somebody's password travels through a chat app, so this is the real
// thing: a one-time link, short-lived, single use.

// ResetWindow is how long a reset link works for. Long enough to find the
// email on another device, short enough that an old message in an inbox is not
// a way in.
const ResetWindow = time.Hour

// ErrResetInvalid covers every reason a link will not work: never existed,
// already used, or too old. They are one message on purpose, so the page
// cannot be used to learn anything about somebody else's account.
var ErrResetInvalid = errors.New("that reset link is no longer valid")

const resetSchema = `
CREATE TABLE IF NOT EXISTS password_resets (
  token_hash  TEXT PRIMARY KEY,
  account_id  TEXT NOT NULL REFERENCES accounts(id),
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  used_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS password_resets_account ON password_resets(account_id);
`

// StartPasswordReset makes a reset token for an email address.
//
// It returns ErrNotFound when nobody has that address. The CALLER must not
// pass that on: the website always says "if that address has an account, we
// have sent a link", because anything else turns this page into a way of
// finding out who has an account.
func (s *Store) StartPasswordReset(email string) (token string, acct Account, err error) {
	acct, err = s.AccountByEmail(email)
	if err != nil {
		return "", Account{}, err
	}
	// Any earlier link is dropped, so the newest email is the only one that
	// works. Two links in an inbox is a puzzle nobody needs.
	if _, err := s.db.Exec(`DELETE FROM password_resets WHERE account_id = ?`, acct.ID); err != nil {
		return "", Account{}, err
	}
	token, hash := NewToken()
	now := s.now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO password_resets (token_hash, account_id, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		hash, acct.ID, ms(now), ms(now.Add(ResetWindow))); err != nil {
		return "", Account{}, err
	}
	return token, acct, nil
}

// FinishPasswordReset sets a new password from a reset token.
//
// Every session is ended as well: if the reset was needed because somebody
// else had got in, leaving their session alive would defeat the whole exercise.
func (s *Store) FinishPasswordReset(token, password string) error {
	if err := CheckPassword(password); err != nil {
		return err
	}
	var accountID string
	var expires, used int64
	err := s.db.QueryRow(
		`SELECT account_id, expires_at, used_at FROM password_resets WHERE token_hash = ?`,
		HashToken(token)).Scan(&accountID, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrResetInvalid
	}
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if used != 0 || now.After(fromMs(expires)) {
		return ErrResetInvalid
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE accounts SET password_hash = ? WHERE id = ?`,
		string(hash), accountID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE password_resets SET used_at = ? WHERE token_hash = ?`,
		ms(now), HashToken(token)); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM sessions WHERE account_id = ?`, accountID)
	return err
}

// CheckPassword is the one place the password rule lives, so signing up and
// resetting cannot drift apart.
func CheckPassword(password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return errors.New("please use a password of at least 8 characters")
	}
	return nil
}
