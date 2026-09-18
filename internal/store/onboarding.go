package store

import "time"

// Onboarding is the gap between signing up and linking a PC.
//
// People find QueueUp on their phone, from a video, usually nowhere near their
// gaming PC. They sign up, see "now do this on your PC", and put the phone
// down. Hours later, at the PC, QueueUp is forgotten. The reminder below is the
// one nudge that closes that gap: sent once, ever, only to somebody who has an
// account and no PC yet, and never after the first week.

// AccountsAwaitingPC lists accounts old enough to have had a chance to link a
// PC, young enough that a reminder is still welcome, that have no PC linked and
// have never been reminded.
func (s *Store) AccountsAwaitingPC(now time.Time, minAge, maxAge time.Duration) ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.email, a.created_at FROM accounts a
		 WHERE a.pc_reminder_at = 0
		   AND a.created_at <= ? AND a.created_at >= ?
		   AND NOT EXISTS (SELECT 1 FROM devices d WHERE d.account_id = a.id AND d.claimed_at != 0)
		 ORDER BY a.created_at`,
		ms(now.Add(-minAge)), ms(now.Add(-maxAge)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var created int64
		if err := rows.Scan(&a.ID, &a.Email, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = fromMs(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkPCReminded records that the one reminder has gone, so it never goes
// again. Called before sending, not after: a crash between the two means one
// missed reminder, which is better than two sent.
func (s *Store) MarkPCReminded(accountID string) error {
	_, err := s.db.Exec(`UPDATE accounts SET pc_reminder_at = ? WHERE id = ?`, ms(s.now().UTC()), accountID)
	return err
}

// HasLinkedPC reports whether an account has a PC linked right now.
func (s *Store) HasLinkedPC(accountID string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM devices WHERE account_id = ? AND claimed_at != 0 AND revoked_at = 0`,
		accountID).Scan(&n)
	return n > 0, err
}
