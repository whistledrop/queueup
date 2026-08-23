// Package store is the relay's memory: accounts, paired PCs, and jobs.
//
// It is SQLite in a single file. That is deliberate. It removes an entire moving
// part from the deployment, it survives a relay restart, and everything in here
// is plain SQL, so moving to Postgres later is a driver change rather than a
// rewrite. The pure-Go SQLite driver means the relay still cross-compiles to any
// host with one command.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned instead of sql.ErrNoRows so callers never import
// database/sql just to check.
var ErrNotFound = errors.New("not found")

// Store is a handle on the relay's database.
type Store struct {
	db *sql.DB
	// now is swappable so tests do not have to sleep.
	now func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL UNIQUE,
  token_hash   TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL
);

-- One row per paired PC. token_hash is a hash, never the token itself: a stolen
-- copy of this database cannot be used to impersonate anyone's agent.
CREATE TABLE IF NOT EXISTS devices (
  id             TEXT PRIMARY KEY,
  account_id     TEXT REFERENCES accounts(id),
  name           TEXT NOT NULL DEFAULT '',
  token_hash     TEXT NOT NULL UNIQUE,
  pairing_code   TEXT,
  claim_hash     TEXT,
  agent_version  TEXT NOT NULL DEFAULT '',
  os             TEXT NOT NULL DEFAULT '',
  hostname       TEXT NOT NULL DEFAULT '',
  simulator      INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL,
  expires_at     INTEGER NOT NULL DEFAULT 0,
  claimed_at     INTEGER NOT NULL DEFAULT 0,
  last_seen_at   INTEGER NOT NULL DEFAULT 0,
  revoked_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS devices_account ON devices(account_id);
CREATE UNIQUE INDEX IF NOT EXISTS devices_code ON devices(pairing_code) WHERE pairing_code IS NOT NULL;

-- A device token is minted at the moment of pairing and parked here until the
-- waiting agent collects it, which it may do exactly once. Nothing else ever
-- reads this table, and the row is deleted on collection.
CREATE TABLE IF NOT EXISTS pending_tokens (
  device_id TEXT PRIMARY KEY REFERENCES devices(id),
  token     TEXT NOT NULL,
  at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
  id                 TEXT PRIMARY KEY,
  account_id         TEXT NOT NULL REFERENCES accounts(id),
  device_id          TEXT NOT NULL REFERENCES devices(id),
  server_addr        TEXT NOT NULL,
  server_name        TEXT NOT NULL DEFAULT '',
  server_id          TEXT NOT NULL DEFAULT '',
  wait_for_server_up INTEGER NOT NULL DEFAULT 0,
  -- group_id is unused in v1. It is here so that clan joins, the flagship v2
  -- feature, do not need a schema migration later.
  group_id           TEXT,
  state              TEXT NOT NULL,
  position           INTEGER NOT NULL DEFAULT 0,
  attempt            INTEGER NOT NULL DEFAULT 0,
  detail             TEXT NOT NULL DEFAULT '',
  reason_code        TEXT NOT NULL DEFAULT '',
  reason_message     TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL,
  updated_at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS jobs_device ON jobs(device_id, state);
CREATE INDEX IF NOT EXISTS jobs_account ON jobs(account_id, created_at);

-- A scheduled join. Times are stored as UTC milliseconds, always. The user's
-- timezone is a display concern that lives entirely in the browser: Logan sets
-- a time from Spain, the PC sits in the UK, and neither fact is allowed to
-- touch what is stored here.
CREATE TABLE IF NOT EXISTS schedules (
  id                 TEXT PRIMARY KEY,
  account_id         TEXT NOT NULL REFERENCES accounts(id),
  device_id          TEXT NOT NULL REFERENCES devices(id),
  server_id          TEXT NOT NULL DEFAULT '',
  server_addr        TEXT NOT NULL DEFAULT '',
  server_name        TEXT NOT NULL DEFAULT '',
  fire_at            INTEGER NOT NULL,
  wait_for_server_up INTEGER NOT NULL DEFAULT 0,
  state              TEXT NOT NULL DEFAULT 'pending',  -- pending | fired | cancelled | failed
  note               TEXT NOT NULL DEFAULT '',
  job_id             TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS schedules_due ON schedules(state, fire_at);
CREATE INDEX IF NOT EXISTS schedules_account ON schedules(account_id, created_at);

-- A browser that asked to receive push notifications. One account can have
-- several (phone and laptop). The endpoint and keys are exactly what the
-- browser's push service handed over; we store nothing else about the device.
CREATE TABLE IF NOT EXISTS push_subscriptions (
  endpoint    TEXT PRIMARY KEY,
  account_id  TEXT NOT NULL REFERENCES accounts(id),
  p256dh      TEXT NOT NULL,
  auth        TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS push_account ON push_subscriptions(account_id);

-- Every state change, kept forever. This is the phone's timeline and the
-- debug view, and it is what lets us work out afterwards what went wrong.
CREATE TABLE IF NOT EXISTS job_events (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id    TEXT NOT NULL REFERENCES jobs(id),
  state     TEXT NOT NULL,
  position  INTEGER NOT NULL DEFAULT 0,
  detail    TEXT NOT NULL DEFAULT '',
  at        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS job_events_job ON job_events(job_id, id);
`

// Open opens (and if needed creates) the database at path. Use ":memory:" in
// tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		// Each in-memory database gets its own name. Without that, every
		// ":memory:" database in the process would be the same one, and two
		// tests running side by side would scribble over each other.
		dsn = "file:" + newID("mem") + "?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// SQLite handles one writer at a time. Keeping a single connection avoids
	// "database is locked" entirely, and the relay's write volume is tiny.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("setting %s: %w", pragma, err)
		}
	}
	for _, s := range []string{schema, authSchema, serversSchema} {
		if _, err := db.Exec(s); err != nil {
			return nil, fmt.Errorf("creating schema: %w", err)
		}
	}
	st := &Store{db: db, now: time.Now}
	if err := st.migrate(); err != nil {
		return nil, err
	}
	return st, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// ---------------------------------------------------------------- helpers

func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("out of randomness: " + err.Error())
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b)
}

// NewToken returns a fresh secret and its hash. Only the hash is ever stored.
func NewToken() (token, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("out of randomness: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken hashes a secret for storage and lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// pairingAlphabet leaves out characters people misread out loud or on a phone
// screen: no O versus 0, no I versus 1, no S versus 5.
const pairingAlphabet = "ABCDEFGHJKLMNPQRTUVWXY2346789"

func newPairingCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic("out of randomness: " + err.Error())
	}
	out := make([]byte, 6)
	for i, v := range b {
		out[i] = pairingAlphabet[int(v)%len(pairingAlphabet)]
	}
	return string(out)
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMs(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

// migrate adds columns that were introduced after a database was first created.
// CREATE TABLE IF NOT EXISTS does nothing to a table that already exists, so new
// columns have to be added explicitly or an upgraded relay would fail against an
// older database file.
func (s *Store) migrate() error {
	for _, m := range []struct{ table, column, definition string }{
		{"accounts", "password_hash", "TEXT NOT NULL DEFAULT ''"},
		{"jobs", "server_id", "TEXT NOT NULL DEFAULT ''"},
	} {
		has, err := s.hasColumn(m.table, m.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := s.db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s %s", m.table, m.column, m.definition)); err != nil {
			return fmt.Errorf("adding %s.%s: %w", m.table, m.column, err)
		}
	}
	return nil
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// DebugDump returns every text value in the database, for tests that assert
// something is NOT stored in readable form. It is not exposed over the API.
func (s *Store) DebugDump() (string, error) {
	var out strings.Builder
	tables, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return "", err
	}
	defer tables.Close()

	var names []string
	for tables.Next() {
		var n string
		if err := tables.Scan(&n); err != nil {
			return "", err
		}
		names = append(names, n)
	}
	if err := tables.Err(); err != nil {
		return "", err
	}

	for _, name := range names {
		rows, err := s.db.Query(`SELECT * FROM ` + name)
		if err != nil {
			return "", err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		for rows.Next() {
			cells := make([]any, len(cols))
			for i := range cells {
				cells[i] = new(sql.NullString)
			}
			if err := rows.Scan(cells...); err != nil {
				rows.Close()
				return "", err
			}
			for _, c := range cells {
				out.WriteString(c.(*sql.NullString).String)
				out.WriteByte('\n')
			}
		}
		rows.Close()
	}
	return out.String(), nil
}

// SetClockForTest replaces the store's idea of "now". Tests only.
func (s *Store) SetClockForTest(now func() time.Time) { s.now = now }
