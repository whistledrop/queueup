package store

import (
	"database/sql"
	"errors"
	"time"

	"queueup/internal/protocol"
)

// Job is one join request as the relay sees it. The relay is the source of
// truth: if the agent, the PC, or the relay itself restarts, this row is what
// everything is rebuilt from.
type Job struct {
	ID         string
	AccountID  string
	DeviceID   string
	ServerAddr string
	ServerName string
	ServerID   string // the search provider's id, which is the canonical identity
	// QueryAddr is where status questions go. Rust answers those on a different
	// port from the one players connect to. Empty means "same as ServerAddr".
	QueryAddr       string
	WaitForServerUp bool
	GroupID         string
	State           string
	Position        int
	Attempt         int
	Detail          string
	ReasonCode      string
	ReasonMessage   string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Active reports whether the agent should still be working on this job.
func (j Job) Active() bool { return j.State != "done" && j.State != "failed" }

// Event is one entry in a job's timeline. The JSON names match the ones the
// live status stream uses, so the web app parses both the same way.
type Event struct {
	ID       int64     `json:"id"`
	JobID    string    `json:"job_id"`
	State    string    `json:"state"`
	Position int       `json:"position"`
	Detail   string    `json:"detail"`
	At       time.Time `json:"at"`
}

// NewJob is what the web app (or a curl command) asks for.
type NewJob struct {
	AccountID       string
	DeviceID        string
	ServerAddr      string
	ServerName      string
	ServerID        string
	QueryAddr       string
	WaitForServerUp bool
	GroupID         string
}

// CreateJob records a job in the "pending" state. It is not dispatched here:
// the caller hands it to the hub, and if the agent is offline the job simply
// stays pending until the agent reconnects and asks for it.
func (s *Store) CreateJob(n NewJob) (Job, error) {
	now := s.now().UTC()
	j := Job{
		ID:              newID("job"),
		AccountID:       n.AccountID,
		DeviceID:        n.DeviceID,
		ServerAddr:      n.ServerAddr,
		ServerName:      n.ServerName,
		ServerID:        n.ServerID,
		QueryAddr:       n.QueryAddr,
		WaitForServerUp: n.WaitForServerUp,
		GroupID:         n.GroupID,
		State:           "pending",
		Detail:          "Waiting to be picked up by your PC.",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	wait := 0
	if j.WaitForServerUp {
		wait = 1
	}
	var group any
	if j.GroupID != "" {
		group = j.GroupID
	}
	_, err := s.db.Exec(`
		INSERT INTO jobs (id, account_id, device_id, server_addr, server_name, server_id,
		                  query_addr, wait_for_server_up, group_id, state, detail,
		                  created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.AccountID, j.DeviceID, j.ServerAddr, j.ServerName, j.ServerID,
		j.QueryAddr, wait, group, j.State, j.Detail, ms(now), ms(now))
	if err != nil {
		return Job{}, err
	}
	if err := s.AppendEvent(j.ID, j.State, 0, j.Detail, now); err != nil {
		return Job{}, err
	}
	return j, nil
}

// JobByID reads one job.
func (s *Store) JobByID(id string) (Job, error) {
	return s.jobWhere(`id = ?`, id)
}

// ActiveJobForDevice is the heart of reboot-resume. When an agent reconnects,
// this is the question it asks: "do I have work outstanding?"
func (s *Store) ActiveJobForDevice(deviceID string) (Job, error) {
	return s.jobWhere(
		`device_id = ? AND state NOT IN ('done','failed') ORDER BY created_at DESC LIMIT 1`,
		deviceID)
}

func (s *Store) jobWhere(where string, arg any) (Job, error) {
	var j Job
	var name, serverID, queryAddr, group, detail, rc, rm sql.NullString
	var wait int
	var created, updated int64
	err := s.db.QueryRow(`
		SELECT id, account_id, device_id, server_addr, server_name, server_id, query_addr,
		       wait_for_server_up, group_id, state, position, attempt, detail,
		       reason_code, reason_message, created_at, updated_at
		  FROM jobs WHERE `+where, arg).
		Scan(&j.ID, &j.AccountID, &j.DeviceID, &j.ServerAddr, &name, &serverID, &queryAddr,
			&wait, &group, &j.State, &j.Position, &j.Attempt, &detail, &rc, &rm, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	j.ServerName, j.ServerID, j.GroupID, j.Detail = name.String, serverID.String, group.String, detail.String
	j.QueryAddr = queryAddr.String
	j.ReasonCode, j.ReasonMessage = rc.String, rm.String
	j.WaitForServerUp = wait == 1
	j.CreatedAt, j.UpdatedAt = fromMs(created), fromMs(updated)
	return j, nil
}

// ApplyStatus records a state change reported by the agent, and appends it to
// the job's timeline. Statuses that arrive after a job has finished are ignored,
// so a late message from a dying agent cannot resurrect a cancelled job.
func (s *Store) ApplyStatus(st protocol.JobStatus) (Job, bool, error) {
	j, err := s.JobByID(st.JobID)
	if err != nil {
		return Job{}, false, err
	}
	if !j.Active() {
		return j, false, nil
	}
	at := st.At
	if at.IsZero() {
		at = s.now().UTC()
	}
	_, err = s.db.Exec(`
		UPDATE jobs SET state = ?, position = ?, attempt = ?, detail = ?,
		                reason_code = ?, reason_message = ?, updated_at = ?
		 WHERE id = ?`,
		st.State, st.Position, st.Attempt, st.Detail,
		st.ReasonCode, st.ReasonMessage, ms(at), st.JobID)
	if err != nil {
		return Job{}, false, err
	}
	if err := s.AppendEvent(st.JobID, st.State, st.Position, st.Detail, at); err != nil {
		return Job{}, false, err
	}
	j.State, j.Position, j.Attempt, j.Detail = st.State, st.Position, st.Attempt, st.Detail
	j.ReasonCode, j.ReasonMessage, j.UpdatedAt = st.ReasonCode, st.ReasonMessage, at
	return j, true, nil
}

// AppendEvent adds a line to a job's timeline without changing its state. Used
// for things that happen to a job rather than inside it, like "your PC went
// offline".
func (s *Store) AppendEvent(jobID, state string, position int, detail string, at time.Time) error {
	if at.IsZero() {
		at = s.now().UTC()
	}
	_, err := s.db.Exec(
		`INSERT INTO job_events (job_id, state, position, detail, at) VALUES (?, ?, ?, ?, ?)`,
		jobID, state, position, detail, ms(at))
	return err
}

// FinishJob closes a job out from the relay's side, for cancellations and for
// giving up on an agent that never came back.
func (s *Store) FinishJob(jobID, state, reasonCode, reasonMessage string) error {
	now := s.now().UTC()
	_, err := s.db.Exec(`
		UPDATE jobs SET state = ?, detail = ?, reason_code = ?, reason_message = ?, updated_at = ?
		 WHERE id = ? AND state NOT IN ('done','failed')`,
		state, reasonMessage, reasonCode, reasonMessage, ms(now), jobID)
	if err != nil {
		return err
	}
	return s.AppendEvent(jobID, state, 0, reasonMessage, now)
}

// Events returns a job's timeline. Pass sinceID to get only what is new, which
// is how the phone's live status screen catches up after a dropped connection.
func (s *Store) Events(jobID string, sinceID int64) ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT id, job_id, state, position, detail, at
		  FROM job_events WHERE job_id = ? AND id > ? ORDER BY id`, jobID, sinceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&e.ID, &e.JobID, &e.State, &e.Position, &e.Detail, &at); err != nil {
			return nil, err
		}
		e.At = fromMs(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecentJobs powers the admin view and the phone's history list.
func (s *Store) RecentJobs(accountID string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id FROM jobs ORDER BY created_at DESC LIMIT ?`
	args := []any{limit}
	if accountID != "" {
		q = `SELECT id FROM jobs WHERE account_id = ? ORDER BY created_at DESC LIMIT ?`
		args = []any{accountID, limit}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(ids))
	for _, id := range ids {
		j, err := s.JobByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// UpdateAddress records a freshly looked-up address for a job. Rust server IPs
// change, so the address is resolved again just before we connect rather than
// trusted from whenever the job was created.
func (s *Store) UpdateAddress(jobID, addr, queryAddr, name string) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET server_addr = ?, query_addr = ?, server_name = ? WHERE id = ?`,
		addr, queryAddr, name, jobID)
	return err
}

// PollAddr is where the watcher should send status queries for this job.
func (j Job) PollAddr() string {
	if j.QueryAddr != "" {
		return j.QueryAddr
	}
	return j.ServerAddr
}
