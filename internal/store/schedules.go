package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Schedule is one planned join. The relay owns these: they fire on the relay's
// clock, so the PC being off or the phone being asleep changes nothing.
type Schedule struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"-"`
	DeviceID        string    `json:"device_id"`
	ServerID        string    `json:"server_id"`
	ServerAddr      string    `json:"server_addr"`
	ServerName      string    `json:"server_name"`
	FireAt          time.Time `json:"fire_at"`
	WaitForServerUp bool      `json:"wait_for_server_up"`
	State           string    `json:"state"` // pending | fired | cancelled | failed
	Note            string    `json:"note"`
	JobID           string    `json:"job_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// NewSchedule is what the web app asks for.
type NewSchedule struct {
	AccountID       string
	DeviceID        string
	ServerID        string
	ServerAddr      string
	ServerName      string
	FireAt          time.Time
	WaitForServerUp bool
}

// CreateSchedule stores a planned join. FireAt must be in the future; the small
// grace period forgives clocks that disagree by a few seconds.
func (s *Store) CreateSchedule(n NewSchedule) (Schedule, error) {
	now := s.now().UTC()
	if n.FireAt.Before(now.Add(-time.Minute)) {
		return Schedule{}, errors.New("that time has already passed")
	}
	if n.FireAt.After(now.Add(31 * 24 * time.Hour)) {
		return Schedule{}, errors.New("that's more than a month away. Schedule something closer to the day")
	}
	sc := Schedule{
		ID:              newID("sched"),
		AccountID:       n.AccountID,
		DeviceID:        n.DeviceID,
		ServerID:        n.ServerID,
		ServerAddr:      n.ServerAddr,
		ServerName:      n.ServerName,
		FireAt:          n.FireAt.UTC(),
		WaitForServerUp: n.WaitForServerUp,
		State:           "pending",
		CreatedAt:       now,
	}
	wait := 0
	if sc.WaitForServerUp {
		wait = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO schedules (id, account_id, device_id, server_id, server_addr, server_name,
		                       fire_at, wait_for_server_up, state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.ID, sc.AccountID, sc.DeviceID, sc.ServerID, sc.ServerAddr, sc.ServerName,
		ms(sc.FireAt), wait, sc.State, ms(now))
	if err != nil {
		return Schedule{}, err
	}
	return sc, nil
}

// DueSchedules returns pending schedules whose time has arrived, and atomically
// marks each one as firing so a schedule can never fire twice, even if two
// copies of the scheduler were somehow running.
func (s *Store) DueSchedules() ([]Schedule, error) {
	now := s.now().UTC()
	rows, err := s.db.Query(
		`SELECT id FROM schedules WHERE state = 'pending' AND fire_at <= ? ORDER BY fire_at`,
		ms(now))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Schedule
	for _, id := range ids {
		res, err := s.db.Exec(
			`UPDATE schedules SET state = 'fired' WHERE id = ? AND state = 'pending'`, id)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // someone else took it
		}
		sc, err := s.ScheduleByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// ScheduleByID reads one schedule.
func (s *Store) ScheduleByID(id string) (Schedule, error) {
	return s.scheduleWhere(`id = ?`, id)
}

func (s *Store) scheduleWhere(where string, args ...any) (Schedule, error) {
	var sc Schedule
	var wait int
	var fire, created int64
	err := s.db.QueryRow(`
		SELECT id, account_id, device_id, server_id, server_addr, server_name,
		       fire_at, wait_for_server_up, state, note, job_id, created_at
		  FROM schedules WHERE `+where, args...).
		Scan(&sc.ID, &sc.AccountID, &sc.DeviceID, &sc.ServerID, &sc.ServerAddr, &sc.ServerName,
			&fire, &wait, &sc.State, &sc.Note, &sc.JobID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}
	sc.WaitForServerUp = wait == 1
	sc.FireAt, sc.CreatedAt = fromMs(fire), fromMs(created)
	return sc, nil
}

// ResolveSchedule records how a fired schedule turned out.
func (s *Store) ResolveSchedule(id, state, note, jobID string) error {
	_, err := s.db.Exec(
		`UPDATE schedules SET state = ?, note = ?, job_id = ? WHERE id = ?`,
		state, note, jobID, id)
	return err
}

// CancelSchedule withdraws a planned join before it fires.
func (s *Store) CancelSchedule(accountID, id string) error {
	res, err := s.db.Exec(
		`UPDATE schedules SET state = 'cancelled', note = 'You cancelled this.'
		 WHERE id = ? AND account_id = ? AND state = 'pending'`, id, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("that schedule has already fired or was cancelled")
	}
	return nil
}

// Schedules lists an account's planned joins, soonest first, then history.
func (s *Store) Schedules(accountID string, limit int) ([]Schedule, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id FROM schedules WHERE account_id = ?
		 ORDER BY CASE state WHEN 'pending' THEN 0 ELSE 1 END, fire_at DESC LIMIT ?`,
		accountID, limit)
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
	out := make([]Schedule, 0, len(ids))
	for _, id := range ids {
		sc, err := s.ScheduleByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	// Pending ones soonest-first reads better than the raw DESC above.
	for i, j := 0, 0; j < len(out); j++ {
		if out[j].State == "pending" {
			out[i], out[j] = out[j], out[i]
			i++
		}
	}
	return out, nil
}

// ActiveJobs lists every job an agent should be working on right now. The
// server watcher polls the target of each one.
func (s *Store) ActiveJobs() ([]Job, error) {
	rows, err := s.db.Query(
		`SELECT id FROM jobs WHERE state NOT IN ('done','failed') ORDER BY created_at`)
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
