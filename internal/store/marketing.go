package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Marketing email: the automatic flows, and campaigns written by hand.
//
// One table remembers every marketing email that has gone to anybody, keyed by
// what it was ("first_join", "joins_out:2026-10", "campaign:cmp_..."). That one
// fact does three jobs. It stops anything going twice, because a send is
// claimed here before it leaves and a key can only be claimed once. It lets a
// campaign resume where it stopped after a restart. And it is the frequency
// cap: somebody who heard from QueueUp yesterday is passed over today, whatever
// would have been next.
//
// As with the win-back emails, nobody is put on a list ahead of time. Each
// email asks, at the moment of sending, whether this person still wants email,
// still has an account and still fits; saying stop or asking to be deleted
// ends everything because the next query simply does not find them.
//
// Leads, addresses typed on the landing page by people who never made an
// account, are never mailed from here. They did not agree to marketing and
// have no unsubscribe link, and an email to them is the kind that gets a
// sending domain blocked.

const marketingSchema = `
CREATE TABLE IF NOT EXISTS marketing_sends (
  account_id TEXT NOT NULL,
  key        TEXT NOT NULL,
  sent_at    INTEGER NOT NULL,
  PRIMARY KEY (account_id, key)
);
CREATE INDEX IF NOT EXISTS marketing_sends_at ON marketing_sends(account_id, sent_at);
CREATE INDEX IF NOT EXISTS marketing_sends_key ON marketing_sends(key, sent_at);

CREATE TABLE IF NOT EXISTS campaigns (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  subject     TEXT NOT NULL,
  body        TEXT NOT NULL,
  segment     TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'draft',
  start_at    INTEGER NOT NULL DEFAULT 0,
  daily_limit INTEGER NOT NULL DEFAULT 200,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
`

// ClaimMarketingSend records that `key` is going to this account, and
// reports whether this call is the one that claimed it. Called BEFORE the
// send: a crash between the two means one email missed, never one sent twice.
func (s *Store) ClaimMarketingSend(accountID, key string) (bool, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO marketing_sends (account_id, key, sent_at) VALUES (?, ?, ?)`,
		accountID, key, ms(s.now().UTC()))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// HasMarketingSend reports whether `key` has ever gone to this account.
func (s *Store) HasMarketingSend(accountID, key string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM marketing_sends WHERE account_id = ? AND key = ?`, accountID, key).Scan(&n)
	return n > 0, err
}

// marketable is what every marketing query starts from: an account that has
// not said stop, is not on its way to deletion, and has not heard from
// QueueUp since `capSince`.
const marketable = `a.email_optout_at = 0 AND a.erase_after = 0
   AND NOT EXISTS (SELECT 1 FROM marketing_sends c WHERE c.account_id = a.id AND c.sent_at > ?)`

const marketingAccountColumns = `a.id, a.email, a.created_at, a.erase_after`

// hasPC is an account with a linked PC that has not been unlinked.
const hasPC = `EXISTS (SELECT 1 FROM devices d WHERE d.account_id = a.id AND d.claimed_at > 0 AND d.revoked_at = 0)`

// onFree is an account with no paid or hand-given access.
const onFree = `a.subscription_status NOT IN ('active', '` + StatusComped + `')`

func (s *Store) marketingAccounts(query string, args ...any) ([]Account, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FirstJoinDue returns accounts whose first successful join finished between
// `until` and `from` ago, and who have not had `key`.
func (s *Store) FirstJoinDue(now time.Time, from, until time.Duration, key string, capSince time.Time) ([]Account, error) {
	return s.marketingAccounts(`
		SELECT `+marketingAccountColumns+` FROM accounts a
		 WHERE `+marketable+`
		   AND NOT EXISTS (SELECT 1 FROM marketing_sends m WHERE m.account_id = a.id AND m.key = ?)
		   AND (SELECT MIN(j.updated_at) FROM jobs j
		         WHERE j.account_id = a.id AND j.state = 'done' AND j.reason_code = '')
		       BETWEEN ? AND ?
		 ORDER BY a.created_at LIMIT 200`,
		ms(capSince), key, ms(now.Add(-until)), ms(now.Add(-from)))
}

// FreeJoinsDue returns free accounts that have used at least `atLeast` and
// fewer than `below` successful joins since monthStart, and have not had `key`.
func (s *Store) FreeJoinsDue(monthStart time.Time, atLeast, below int, key string, capSince time.Time) ([]Account, error) {
	return s.marketingAccounts(`
		SELECT `+marketingAccountColumns+` FROM accounts a
		 WHERE `+marketable+` AND `+onFree+`
		   AND NOT EXISTS (SELECT 1 FROM marketing_sends m WHERE m.account_id = a.id AND m.key = ?)
		   AND (SELECT COUNT(*) FROM jobs j
		         WHERE j.account_id = a.id AND j.created_at >= ? AND j.state = 'done'
		           AND j.reason_code = '' AND (j.group_id IS NULL OR j.group_id = ''))
		       BETWEEN ? AND ?
		 ORDER BY a.created_at LIMIT 200`,
		ms(capSince), key, ms(monthStart), atLeast, below-1)
}

// EveryoneDue returns every marketable account that has not had `key`, for
// the monthly force wipe email.
func (s *Store) EveryoneDue(key string, capSince time.Time, limit int) ([]Account, error) {
	return s.marketingAccounts(`
		SELECT `+marketingAccountColumns+` FROM accounts a
		 WHERE `+marketable+`
		   AND NOT EXISTS (SELECT 1 FROM marketing_sends m WHERE m.account_id = a.id AND m.key = ?)
		 ORDER BY a.created_at LIMIT ?`,
		ms(capSince), key, limit)
}

// ------------------------------------------------------------- campaigns

// Segments a campaign can go to. Each is a plain description of who, and the
// SQL that finds them; adding one is adding a line here.
var Segments = []Segment{
	{ID: "everyone", Name: "Everyone", Where: `1 = 1`},
	{ID: "free", Name: "Free plan", Where: onFree},
	{ID: "free_with_pc", Name: "Free plan, PC linked", Where: onFree + ` AND ` + hasPC},
	{ID: "free_no_pc", Name: "Free plan, no PC linked yet", Where: onFree + ` AND NOT ` + hasPC},
	{ID: "paid", Name: "Paying subscribers", Where: `a.subscription_status = 'active'`},
	{ID: "lapsed", Name: "Used to pay, not now", Where: `a.first_paid_at > 0 AND ` + onFree},
}

// Segment is one audience a campaign can be sent to.
type Segment struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Where string `json:"-"`
}

func segmentByID(id string) (Segment, bool) {
	for _, s := range Segments {
		if s.ID == id {
			return s, true
		}
	}
	return Segment{}, false
}

// Campaign statuses.
const (
	CampaignDraft     = "draft"
	CampaignScheduled = "scheduled"
	CampaignDone      = "done"
	CampaignCancelled = "cancelled"
)

// Campaign is one email written by hand, sent to one segment, a daily
// allowance at a time.
type Campaign struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	Segment    string    `json:"segment"`
	Status     string    `json:"status"`
	StartAt    time.Time `json:"start_at"`
	DailyLimit int       `json:"daily_limit"`
	CreatedAt  time.Time `json:"created_at"`
	// Sent is how many have gone so far. Filled in by ListCampaigns.
	Sent int `json:"sent"`
}

// SendKey is what a campaign's sends are recorded under.
func (c Campaign) SendKey() string { return "campaign:" + c.ID }

// ErrBadCampaign is a campaign that cannot be saved as given.
var ErrBadCampaign = errors.New("campaign is not valid")

func checkCampaign(c *Campaign) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Subject = strings.TrimSpace(c.Subject)
	c.Body = strings.TrimSpace(c.Body)
	switch {
	case c.Subject == "":
		return fmt.Errorf("%w: it needs a subject", ErrBadCampaign)
	case len(c.Subject) > 150:
		return fmt.Errorf("%w: the subject is too long", ErrBadCampaign)
	case c.Body == "":
		return fmt.Errorf("%w: it needs something to say", ErrBadCampaign)
	case len(c.Body) > 20000:
		return fmt.Errorf("%w: the email is too long", ErrBadCampaign)
	}
	if _, ok := segmentByID(c.Segment); !ok {
		return fmt.Errorf("%w: unknown audience %q", ErrBadCampaign, c.Segment)
	}
	if c.Name == "" {
		c.Name = c.Subject
	}
	// A new sending domain that suddenly sends thousands looks exactly like
	// spam to Gmail. A daily allowance spreads a big send over days.
	if c.DailyLimit <= 0 {
		c.DailyLimit = 200
	}
	if c.DailyLimit > 5000 {
		c.DailyLimit = 5000
	}
	return nil
}

// CreateCampaign saves a new draft.
func (s *Store) CreateCampaign(c Campaign) (Campaign, error) {
	if err := checkCampaign(&c); err != nil {
		return Campaign{}, err
	}
	now := s.now().UTC()
	c.ID, c.Status, c.CreatedAt = newID("cmp"), CampaignDraft, now
	_, err := s.db.Exec(
		`INSERT INTO campaigns (id, name, subject, body, segment, status, daily_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Subject, c.Body, c.Segment, c.Status, c.DailyLimit, ms(now), ms(now))
	return c, err
}

// ErrCampaignLocked is an edit to a campaign that has started sending.
var ErrCampaignLocked = errors.New("only a draft can be changed")

// UpdateCampaign rewrites a draft. Once a campaign is scheduled its words are
// fixed: half the audience reading one email and half another is a mess
// nobody can reason about afterwards.
func (s *Store) UpdateCampaign(c Campaign) (Campaign, error) {
	if err := checkCampaign(&c); err != nil {
		return Campaign{}, err
	}
	res, err := s.db.Exec(
		`UPDATE campaigns SET name = ?, subject = ?, body = ?, segment = ?, daily_limit = ?, updated_at = ?
		  WHERE id = ? AND status = ?`,
		c.Name, c.Subject, c.Body, c.Segment, c.DailyLimit, ms(s.now().UTC()), c.ID, CampaignDraft)
	if err != nil {
		return Campaign{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.CampaignByID(c.ID); err != nil {
			return Campaign{}, err
		}
		return Campaign{}, ErrCampaignLocked
	}
	return s.CampaignByID(c.ID)
}

const campaignColumns = `id, name, subject, body, segment, status, start_at, daily_limit, created_at`

func scanCampaign(row scannable) (Campaign, error) {
	var c Campaign
	var start, created int64
	if err := row.Scan(&c.ID, &c.Name, &c.Subject, &c.Body, &c.Segment, &c.Status, &start, &c.DailyLimit, &created); err != nil {
		return Campaign{}, err
	}
	c.StartAt, c.CreatedAt = fromMs(start), fromMs(created)
	return c, nil
}

// CampaignByID reads one campaign.
func (s *Store) CampaignByID(id string) (Campaign, error) {
	c, err := scanCampaign(s.db.QueryRow(`SELECT `+campaignColumns+` FROM campaigns WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	return c, err
}

// ListCampaigns returns every campaign, newest first, with how many have gone.
func (s *Store) ListCampaigns() ([]Campaign, error) {
	rows, err := s.db.Query(`SELECT ` + campaignColumns + ` FROM campaigns ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Sent, err = s.CampaignSentSince(out[i], time.Time{}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ScheduleCampaign starts a draft sending from `at`.
func (s *Store) ScheduleCampaign(id string, at time.Time) error {
	return s.moveCampaign(id, CampaignScheduled, at, CampaignDraft)
}

// CancelCampaign stops a draft or a campaign part way through. What has gone
// has gone; nothing more will.
func (s *Store) CancelCampaign(id string) error {
	return s.moveCampaign(id, CampaignCancelled, time.Time{}, CampaignDraft, CampaignScheduled)
}

// FinishCampaign marks a campaign that has reached everybody it can.
func (s *Store) FinishCampaign(id string) error {
	return s.moveCampaign(id, CampaignDone, time.Time{}, CampaignScheduled)
}

func (s *Store) moveCampaign(id, to string, at time.Time, from ...string) error {
	q := `UPDATE campaigns SET status = ?, updated_at = ?`
	args := []any{to, ms(s.now().UTC())}
	if !at.IsZero() {
		q += `, start_at = ?`
		args = append(args, ms(at.UTC()))
	}
	q += ` WHERE id = ? AND status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(from)), ",") + `)`
	args = append(args, id)
	for _, f := range from {
		args = append(args, f)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.CampaignByID(id); err != nil {
			return err
		}
		return fmt.Errorf("that campaign can't be moved to %s from where it is", to)
	}
	return nil
}

// DueCampaigns returns scheduled campaigns whose start time has come.
func (s *Store) DueCampaigns(now time.Time) ([]Campaign, error) {
	rows, err := s.db.Query(`SELECT `+campaignColumns+` FROM campaigns WHERE status = ? AND start_at <= ? ORDER BY start_at`,
		CampaignScheduled, ms(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CampaignSentSince counts this campaign's sends since `since`.
func (s *Store) CampaignSentSince(c Campaign, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM marketing_sends WHERE key = ? AND sent_at >= ?`,
		c.SendKey(), ms(since)).Scan(&n)
	return n, err
}

// CampaignRecipients returns up to `limit` accounts in the campaign's segment
// that have not had it, skipping anybody who heard from QueueUp since
// capSince; they are picked up on a later round.
func (s *Store) CampaignRecipients(c Campaign, limit int, capSince time.Time) ([]Account, error) {
	seg, ok := segmentByID(c.Segment)
	if !ok {
		return nil, ErrBadCampaign
	}
	return s.marketingAccounts(`
		SELECT `+marketingAccountColumns+` FROM accounts a
		 WHERE `+marketable+` AND (`+seg.Where+`)
		   AND NOT EXISTS (SELECT 1 FROM marketing_sends m WHERE m.account_id = a.id AND m.key = ?)
		 ORDER BY a.created_at LIMIT ?`,
		ms(capSince), c.SendKey(), limit)
}

// CampaignRemaining counts who in the segment has still not had the campaign,
// ignoring the frequency cap: zero means it is finished.
func (s *Store) CampaignRemaining(c Campaign) (int, error) {
	seg, ok := segmentByID(c.Segment)
	if !ok {
		return 0, ErrBadCampaign
	}
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM accounts a
		 WHERE a.email_optout_at = 0 AND a.erase_after = 0 AND (`+seg.Where+`)
		   AND NOT EXISTS (SELECT 1 FROM marketing_sends m WHERE m.account_id = a.id AND m.key = ?)`,
		c.SendKey()).Scan(&n)
	return n, err
}

// SegmentSize counts who a segment would reach today.
func (s *Store) SegmentSize(segmentID string) (int, error) {
	seg, ok := segmentByID(segmentID)
	if !ok {
		return 0, ErrBadCampaign
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts a WHERE a.email_optout_at = 0 AND a.erase_after = 0 AND (` +
		seg.Where + `)`).Scan(&n)
	return n, err
}

// MarketingCounts is how many of each automatic email went out since `since`,
// keyed by the email's name without its month ("joins_out", "wipe_week"...).
func (s *Store) MarketingCounts(since time.Time) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT key, COUNT(*) FROM marketing_sends WHERE sent_at >= ? AND key NOT LIKE 'campaign:%' GROUP BY key`,
		ms(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		if i := strings.IndexByte(key, ':'); i > 0 {
			key = key[:i]
		}
		out[key] += n
	}
	return out, rows.Err()
}
