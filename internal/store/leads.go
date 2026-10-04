package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Leads: email addresses from people who started signing up.
//
// The address is taken at the first step, before the password, so somebody who
// types their email and then thinks better of it still leaves a trace. Those
// are the most interesting people we have: interested enough to click a link
// and type an address, and then something stopped them. Without this they were
// invisible, and you cannot fix a drop-off you cannot see.
//
// A lead is NOT a customer and is never treated as one. It carries what they
// gave us and the code they arrived on, and nothing else.

const leadSchema = `
CREATE TABLE IF NOT EXISTS leads (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL UNIQUE,
  source_code  TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  -- When an account with this address appeared. Zero while they are still
  -- only a lead.
  converted_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS leads_created ON leads(created_at);
`

// RecordLead keeps an address somebody typed on their way in.
//
// Typing it again later updates the code they arrived on but never the date:
// the first time somebody showed up is the fact worth keeping.
func (s *Store) RecordLead(email, sourceCode string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(email) < 5 || len(email) > 320 {
		return errors.New("that doesn't look like an email address")
	}
	sourceCode = strings.ToUpper(strings.TrimSpace(sourceCode))
	if len(sourceCode) > 64 {
		sourceCode = sourceCode[:64]
	}
	now := ms(s.now().UTC())
	_, err := s.db.Exec(`
		INSERT INTO leads (id, email, source_code, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
		  source_code = CASE WHEN excluded.source_code != '' THEN excluded.source_code ELSE leads.source_code END`,
		newID("lead"), email, sourceCode, now)
	return err
}

// MarkLeadConverted notes that this address now has an account behind it.
func (s *Store) MarkLeadConverted(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	_, err := s.db.Exec(
		`UPDATE leads SET converted_at = ? WHERE email = ? AND converted_at = 0`,
		ms(s.now().UTC()), email)
	return err
}

// Stage is where one person has got to. The order here is the order of the
// funnel, and every person is in exactly one of them.
type Stage string

const (
	// StageLead gave an email address and never came back.
	StageLead Stage = "lead"
	// StageSignedUp has an account and nothing else.
	StageSignedUp Stage = "signed_up"
	// StagePaying is a customer right now.
	StagePaying Stage = "paying"
	// StageLapsed paid once and stopped. Deliberately NOT the same as
	// never having paid: one is a product problem, the other is a pitch
	// problem, and they need opposite work.
	StageLapsed Stage = "lapsed"
	// StageFree was given access by hand.
	StageFree Stage = "free"
)

// Person is one row of the funnel, lead or account alike.
type Person struct {
	Email      string
	Stage      Stage
	SourceCode string
	HasPC      bool
	Joins      int
	CreatedAt  time.Time
	FirstPaid  time.Time
}

// Funnel lists everybody we have an address for, newest first: the people who
// only ever gave an email alongside the people who went all the way.
func (s *Store) Funnel() ([]Person, error) {
	out := []Person{}

	rows, err := s.db.Query(`
		SELECT a.email,
		       COALESCE(a.subscription_status, 'none'),
		       COALESCE(a.source_code, ''),
		       COALESCE(a.first_paid_at, 0),
		       a.created_at,
		       (SELECT COUNT(*) FROM devices d WHERE d.account_id = a.id AND d.claimed_at != 0 AND d.revoked_at = 0),
		       (SELECT COUNT(*) FROM jobs j WHERE j.account_id = a.id)
		  FROM accounts a
		 ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Person
		var status string
		var paid, created int64
		var devices int
		if err := rows.Scan(&p.Email, &status, &p.SourceCode, &paid, &created, &devices, &p.Joins); err != nil {
			return nil, err
		}
		p.CreatedAt = fromMs(created)
		p.FirstPaid = fromMs(paid)
		p.HasPC = devices > 0
		switch {
		case status == StatusComped:
			p.Stage = StageFree
		case status == "active":
			p.Stage = StagePaying
		case paid != 0:
			p.Stage = StageLapsed
		default:
			p.Stage = StageSignedUp
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Then the ones who never got as far as an account.
	leadRows, err := s.db.Query(`
		SELECT email, source_code, created_at FROM leads
		 WHERE converted_at = 0
		   AND email NOT IN (SELECT email FROM accounts)
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer leadRows.Close()
	for leadRows.Next() {
		var p Person
		var created int64
		if err := leadRows.Scan(&p.Email, &p.SourceCode, &created); err != nil {
			return nil, err
		}
		p.CreatedAt = fromMs(created)
		p.Stage = StageLead
		out = append(out, p)
	}
	return out, leadRows.Err()
}

var _ = sql.ErrNoRows
