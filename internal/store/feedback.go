package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Feedback is what beta testers tell us: a few words typed on the website, or a
// problem report sent straight from the tray icon on their PC.
//
// Both land in one table because they answer the same question, "how is this
// going for people", and the operator reads them in one list. A report carries
// a body (the agent's log and the tail of the game's log); typed feedback does
// not.
type Feedback struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	Email        string    `json:"email"`
	DeviceID     string    `json:"device_id,omitempty"`
	DeviceName   string    `json:"device_name,omitempty"`
	Kind         string    `json:"kind"` // FeedbackTyped or FeedbackReport
	Message      string    `json:"message"`
	AgentVersion string    `json:"agent_version,omitempty"`
	BodyBytes    int       `json:"body_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

const (
	FeedbackTyped  = "feedback"
	FeedbackReport = "report"
)

// Limits. A message is a few sentences, not an essay; a report is two log tails
// of at most half a megabyte each, plus a little header.
const (
	MaxFeedbackMessage = 4000
	MaxReportBody      = 2 << 20
)

const feedbackSchema = `
CREATE TABLE IF NOT EXISTS feedback (
  id             TEXT PRIMARY KEY,
  account_id     TEXT NOT NULL,
  device_id      TEXT NOT NULL DEFAULT '',
  kind           TEXT NOT NULL,
  message        TEXT NOT NULL DEFAULT '',
  body           TEXT NOT NULL DEFAULT '',
  agent_version  TEXT NOT NULL DEFAULT '',
  created_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS feedback_created ON feedback(created_at);
`

// NewFeedback is what a caller hands over. The store fills in the rest.
type NewFeedback struct {
	AccountID    string
	DeviceID     string
	Kind         string
	Message      string
	Body         string
	AgentVersion string
}

// AddFeedback stores one piece of feedback or one problem report.
func (s *Store) AddFeedback(n NewFeedback) (Feedback, error) {
	n.Message = strings.TrimSpace(n.Message)
	switch n.Kind {
	case FeedbackTyped:
		if n.Message == "" {
			return Feedback{}, errors.New("write a few words first")
		}
		n.Body = ""
	case FeedbackReport:
		if strings.TrimSpace(n.Body) == "" {
			return Feedback{}, errors.New("the report was empty")
		}
	default:
		return Feedback{}, errors.New("unknown kind of feedback")
	}
	if utf8.RuneCountInString(n.Message) > MaxFeedbackMessage {
		return Feedback{}, errors.New("that is a bit long: keep it under 4000 characters")
	}
	if len(n.Body) > MaxReportBody {
		return Feedback{}, errors.New("that report is too large to send")
	}
	if n.AccountID == "" {
		return Feedback{}, errors.New("feedback needs an account")
	}

	f := Feedback{
		ID: newID("fb"), AccountID: n.AccountID, DeviceID: n.DeviceID,
		Kind: n.Kind, Message: n.Message, AgentVersion: n.AgentVersion,
		BodyBytes: len(n.Body), CreatedAt: s.now().UTC(),
	}
	_, err := s.db.Exec(
		`INSERT INTO feedback (id, account_id, device_id, kind, message, body, agent_version, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.AccountID, f.DeviceID, f.Kind, f.Message, n.Body, f.AgentVersion, ms(f.CreatedAt))
	if err != nil {
		return Feedback{}, err
	}
	return f, nil
}

// RecentFeedback lists the newest feedback first, without report bodies: a
// list screen has no business downloading megabytes of logs it may never open.
func (s *Store) RecentFeedback(limit int) ([]Feedback, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT f.id, f.account_id, COALESCE(a.email, ''), f.device_id,
		       COALESCE(d.name, ''), f.kind, f.message, f.agent_version,
		       LENGTH(f.body), f.created_at
		  FROM feedback f
		  LEFT JOIN accounts a ON a.id = f.account_id
		  LEFT JOIN devices d ON d.id = f.device_id
		 ORDER BY f.created_at DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		var f Feedback
		var created int64
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Email, &f.DeviceID, &f.DeviceName,
			&f.Kind, &f.Message, &f.AgentVersion, &f.BodyBytes, &created); err != nil {
			return nil, err
		}
		f.CreatedAt = fromMs(created)
		out = append(out, f)
	}
	return out, rows.Err()
}

// FeedbackBody returns a problem report's contents.
func (s *Store) FeedbackBody(id string) (string, error) {
	var body string
	err := s.db.QueryRow(`SELECT body FROM feedback WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return body, err
}

// JobOutcome is one line of "how did joins go": how many ended in this state,
// for this reason.
type JobOutcome struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// JobOutcomes counts joins created since the given time by where they ended up.
// On wipe day this is the one number that says whether the product is working.
func (s *Store) JobOutcomes(since time.Time) ([]JobOutcome, error) {
	rows, err := s.db.Query(`
		SELECT state, reason_code, COUNT(*)
		  FROM jobs
		 WHERE created_at >= ?
		 GROUP BY state, reason_code
		 ORDER BY COUNT(*) DESC`, ms(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobOutcome{}
	for rows.Next() {
		var o JobOutcome
		if err := rows.Scan(&o.State, &o.Reason, &o.Count); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Failure is a join that did not work, with who it belonged to, so the operator
// can go and ask them about it.
type Failure struct {
	JobID      string    `json:"job_id"`
	Email      string    `json:"email"`
	ServerName string    `json:"server_name"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	Attempts   int       `json:"attempts"`
	At         time.Time `json:"at"`
}

// RecentFailures lists failed joins since the given time, newest first.
func (s *Store) RecentFailures(since time.Time, limit int) ([]Failure, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT j.id, COALESCE(a.email, ''), COALESCE(NULLIF(j.server_name, ''), j.server_addr),
		       j.reason_code, j.reason_message, j.attempt, j.updated_at
		  FROM jobs j
		  LEFT JOIN accounts a ON a.id = j.account_id
		 WHERE j.state = 'failed' AND j.created_at >= ?
		 ORDER BY j.updated_at DESC
		 LIMIT ?`, ms(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Failure{}
	for rows.Next() {
		var f Failure
		var at int64
		if err := rows.Scan(&f.JobID, &f.Email, &f.ServerName, &f.Reason, &f.Message, &f.Attempts, &at); err != nil {
			return nil, err
		}
		f.At = fromMs(at)
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFeedback removes one piece of feedback or one problem report. Reports
// hold people's logs, so they are not kept once they have served their purpose.
func (s *Store) DeleteFeedback(id string) error {
	res, err := s.db.Exec(`DELETE FROM feedback WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EraseAccount deletes an account and everything that belongs to it: PCs,
// joins and their timelines, schedules, saved servers, sessions, feedback and
// problem reports. It is the "delete everything you hold about me" request,
// and it cannot be undone.
//
// Unlike DeleteAccount, which refuses anything with history, this is meant for
// real people who have asked. It finds the tables to clear by looking at the
// database rather than from a list, so a table left behind by an older version
// (the notification subscriptions, removed from the code but still present in
// a long-lived database) cannot block it or survive it.
func (s *Store) EraseAccount(accountID string) error {
	if _, err := s.AccountByID(accountID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	tables, err := tablesWithColumns(tx)
	if err != nil {
		return err
	}
	const accountJobs = `SELECT id FROM jobs WHERE account_id = ?`
	const accountDevices = `SELECT id FROM devices WHERE account_id = ?`

	// Children first, so no foreign key is ever left pointing at nothing.
	stmts := []string{`DELETE FROM job_events WHERE job_id IN (` + accountJobs + `)`}
	for name, cols := range tables {
		switch name {
		case "accounts", "devices", "jobs", "job_events":
			continue
		}
		if cols["account_id"] {
			stmts = append(stmts, `DELETE FROM "`+name+`" WHERE account_id = ?`)
		}
		if cols["device_id"] {
			stmts = append(stmts, `DELETE FROM "`+name+`" WHERE device_id IN (`+accountDevices+`)`)
		}
	}
	stmts = append(stmts,
		`DELETE FROM jobs WHERE account_id = ?`,
		`DELETE FROM devices WHERE account_id = ?`,
		`DELETE FROM accounts WHERE id = ?`,
	)
	for _, q := range stmts {
		if _, err := tx.Exec(q, accountID); err != nil {
			return fmt.Errorf("erasing the account: %w", err)
		}
	}
	return tx.Commit()
}

// tablesWithColumns maps every table to the set of its column names.
func tablesWithColumns(tx *sql.Tx) (map[string]map[string]bool, error) {
	rows, err := tx.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()

	out := map[string]map[string]bool{}
	for _, n := range names {
		cols, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, n)
		if err != nil {
			return nil, err
		}
		set := map[string]bool{}
		for cols.Next() {
			var c string
			if err := cols.Scan(&c); err != nil {
				cols.Close()
				return nil, err
			}
			set[c] = true
		}
		cols.Close()
		out[n] = set
	}
	return out, nil
}
