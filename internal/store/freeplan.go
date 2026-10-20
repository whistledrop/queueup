package store

import "time"

// The free plan.
//
// Everybody gets ten joins a calendar month without paying, plus one scheduled
// wipe join, ever, as a taste of the paid feature. The paid plan is unlimited.
//
// What counts, and what deliberately does not:
//
//   - A join counts only when the player started it AND it worked. Starting a
//     join that fails costs nothing. Charging for failure would be charging for
//     our own bad night: a server that never came back up, a Steam outage, a
//     crash. The player would be out of joins with nothing to show for it.
//   - Retries, reconnects after a reboot, and rejoins after Rust crashes all
//     happen INSIDE one job row. The agent bumps jobs.attempt and the relay
//     hands the same job back; no new row is ever written. So counting rows
//     counts joins the player asked for, and nothing else, with no extra
//     bookkeeping.
//
// The count is derived from the jobs table rather than kept as a running total
// on the account. A stored counter is one more thing to drift: a failed write,
// a restored backup, or a bug and somebody is wrongly locked out of a product
// they are entitled to. The jobs table already holds the truth, and
// jobs_account(account_id, created_at) already indexes it.
const FreeJoinsPerMonth = 10

// MonthStart is the first instant of the calendar month containing t, in UTC.
//
// UTC, not the player's timezone, because the relay's clock is the only clock
// involved and a month boundary that moves with whoever is asking would let the
// same person cross it twice. The cost is that somebody in New Zealand gets
// their new joins late on the 1st by their reckoning; the alternative is a
// reset that can be gamed by changing a phone's timezone.
func MonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// SuccessfulJoinsSince counts the joins this account actually got into a
// server with, since the given instant.
//
// "done with no reason code" is the only outcome the state machine calls a win:
// every other ending (cancelled, player_closed, player_left, server_full,
// game_crashed, gave_up and the rest) carries a reason, so none of them are
// counted here.
//
// Jobs that belong to a group are excluded. Squad joins can only be started by
// a paying member, so every job carrying a group id was paid for by somebody;
// a free teammate pulled into a paid squad join must not have it taken off
// their ten. Nothing writes group_id today, so this excludes nothing yet, and
// starts working by itself the day squad mode lands.
func (s *Store) SuccessfulJoinsSince(accountID string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM jobs
		  WHERE account_id = ?
		    AND created_at >= ?
		    AND state = 'done'
		    AND reason_code = ''
		    AND (group_id IS NULL OR group_id = '')`,
		accountID, ms(since)).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// FreeWipeJoinUsedAt is when this account spent its one free scheduled wipe
// join, or the zero time if it still has it.
//
// Once ever, not once a month: it exists to show somebody what the paid feature
// feels like on the one night it matters, and a monthly free wipe join would be
// most of the reason to pay.
func (s *Store) FreeWipeJoinUsedAt(accountID string) (time.Time, error) {
	var at int64
	err := s.db.QueryRow(
		`SELECT free_wipe_join_used_at FROM accounts WHERE id = ?`, accountID).Scan(&at)
	if err != nil {
		return time.Time{}, err
	}
	return fromMs(at), nil
}

// MarkFreeWipeJoinUsed spends the account's one free scheduled wipe join.
//
// The write refuses to overwrite a time that is already there, so a double
// request cannot hand out a second one or move the date of the first.
func (s *Store) MarkFreeWipeJoinUsed(accountID string, at time.Time) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET free_wipe_join_used_at = ?
		  WHERE id = ? AND free_wipe_join_used_at = 0`,
		ms(at), accountID)
	return err
}
