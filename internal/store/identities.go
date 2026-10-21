package store

import (
	"database/sql"
	"errors"
)

// Sign-in identities: "this Google account" or "this Apple ID" belongs to this
// QueueUp account.
//
// Keyed by the provider's own permanent id for the person, never by email:
// somebody can change the address on their Google account, and an Apple
// "Hide my email" address is different for every app. The email is kept only
// as a note of what it was when they linked.
//
// An identity is as good as a password for every purpose that asks "can this
// person sign in": an account with one is never handed out for an email typed
// on the landing page, which only ever opens accounts nobody can sign in to.

const identitySchema = `
CREATE TABLE IF NOT EXISTS account_identities (
  provider   TEXT NOT NULL,
  subject    TEXT NOT NULL,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  email      TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (provider, subject)
);
CREATE INDEX IF NOT EXISTS account_identities_account ON account_identities(account_id);
`

// AccountByIdentity finds the account a provider's person is linked to.
func (s *Store) AccountByIdentity(provider, subject string) (Account, error) {
	var id string
	err := s.db.QueryRow(`SELECT account_id FROM account_identities WHERE provider = ? AND subject = ?`,
		provider, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return s.AccountByID(id)
}

// LinkIdentity ties a provider's person to an account. Linking the same person
// again is a no-op; one person can never be linked to two accounts.
func (s *Store) LinkIdentity(accountID, provider, subject, email string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO account_identities (provider, subject, account_id, email, created_at)
		 VALUES (?, ?, ?, ?, ?)`, provider, subject, accountID, email, ms(s.now().UTC()))
	return err
}

// SignInMethods lists how this account can be signed in to: "password",
// "google", "apple". Empty means nobody can, which is an account started from
// an email alone.
func (s *Store) SignInMethods(accountID string) ([]string, error) {
	var out []string
	if has, err := s.HasPassword(accountID); err != nil {
		return nil, err
	} else if has {
		out = append(out, "password")
	}
	rows, err := s.db.Query(`SELECT DISTINCT provider FROM account_identities WHERE account_id = ? ORDER BY provider`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CanSignIn reports whether the account has any way in: a password or a
// linked identity.
func (s *Store) CanSignIn(accountID string) (bool, error) {
	m, err := s.SignInMethods(accountID)
	return len(m) > 0, err
}
