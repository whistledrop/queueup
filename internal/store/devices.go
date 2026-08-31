package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PairingWindow is how long a pairing code stays usable. Short on purpose: the
// code is displayed on a screen in someone's house, and it should stop working
// long before anyone else could stumble on it.
const PairingWindow = 10 * time.Minute

// Device is one paired PC.
type Device struct {
	ID           string
	AccountID    string
	Name         string
	AgentVersion string
	OS           string
	Hostname     string
	Simulator    bool
	// SleepAfter is minutes until this PC sleeps on mains power: 0 never,
	// -1 unknown.
	SleepAfter int
	CreatedAt  time.Time
	ClaimedAt  time.Time
	LastSeenAt time.Time
	RevokedAt  time.Time
}

// Paired reports whether this device has been linked to an account.
func (d Device) Paired() bool { return d.AccountID != "" && !d.ClaimedAt.IsZero() }

// Revoked reports whether the user has unlinked this PC.
func (d Device) Revoked() bool { return !d.RevokedAt.IsZero() }

// Pairing is what the agent gets when it asks to be paired: a code to show the
// user, and a private token it polls with while it waits.
type Pairing struct {
	DeviceID   string
	Code       string
	ClaimToken string
	ExpiresAt  time.Time
}

// StartPairing creates an unclaimed device and its one-time code. No account is
// involved yet: the agent does this before anybody has typed anything.
func (s *Store) StartPairing(name string) (Pairing, error) {
	claimToken, claimHash := NewToken()
	_, deviceHash := NewToken() // replaced with the real device token on claim
	now := s.now().UTC()
	p := Pairing{
		DeviceID:   newID("dev"),
		Code:       newPairingCode(),
		ClaimToken: claimToken,
		ExpiresAt:  now.Add(PairingWindow),
	}
	_, err := s.db.Exec(`
		INSERT INTO devices (id, name, token_hash, pairing_code, claim_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.DeviceID, name, deviceHash, p.Code, claimHash, ms(now), ms(p.ExpiresAt))
	if err != nil {
		return Pairing{}, err
	}
	return p, nil
}

// ClaimPairingCode links a waiting device to an account. This is what happens
// when the user types the code the agent is showing into the web app.
func (s *Store) ClaimPairingCode(accountID, code string) (Device, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	now := s.now().UTC()

	var id string
	var expires, claimed int64
	err := s.db.QueryRow(
		`SELECT id, expires_at, claimed_at FROM devices WHERE pairing_code = ?`, code).
		Scan(&id, &expires, &claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, fmt.Errorf("that code isn't valid. Check the code showing on your PC and try again")
	}
	if err != nil {
		return Device{}, err
	}
	if claimed != 0 {
		return Device{}, fmt.Errorf("that code has already been used")
	}
	if now.After(fromMs(expires)) {
		return Device{}, fmt.Errorf("that code has expired. Your PC will be showing a new one")
	}

	// The real device token is minted here, at the moment of pairing, and handed
	// to the agent when it next polls.
	token, hash := NewToken()
	if _, err := s.db.Exec(`
		UPDATE devices
		   SET account_id = ?, token_hash = ?, claimed_at = ?, pairing_code = NULL
		 WHERE id = ? AND claimed_at = 0`,
		accountID, hash, ms(now), id); err != nil {
		return Device{}, err
	}
	if err := s.stashDeviceToken(id, token); err != nil {
		return Device{}, err
	}
	return s.DeviceByID(id)
}

// stashDeviceToken parks the freshly minted token where the waiting agent can
// collect it exactly once, then it is deleted.
func (s *Store) stashDeviceToken(deviceID, token string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pending_tokens (device_id, token, at) VALUES (?, ?, ?)`,
		deviceID, token, ms(s.now().UTC()))
	return err
}

// CollectPairingResult is polled by the waiting agent. Once it returns a token,
// that token is deleted from the database and can never be read again.
func (s *Store) CollectPairingResult(claimToken string) (deviceToken string, done bool, err error) {
	var deviceID string
	var claimed, expires int64
	err = s.db.QueryRow(
		`SELECT id, claimed_at, expires_at FROM devices WHERE claim_hash = ?`,
		HashToken(claimToken)).Scan(&deviceID, &claimed, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrNotFound
	}
	if err != nil {
		return "", false, err
	}
	if claimed == 0 {
		if s.now().UTC().After(fromMs(expires)) {
			return "", false, fmt.Errorf("this pairing code expired before anyone used it")
		}
		return "", false, nil
	}

	err = s.db.QueryRow(`SELECT token FROM pending_tokens WHERE device_id = ?`, deviceID).Scan(&deviceToken)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("this pairing has already been completed on another agent")
	}
	if err != nil {
		return "", false, err
	}
	if _, err := s.db.Exec(`DELETE FROM pending_tokens WHERE device_id = ?`, deviceID); err != nil {
		return "", false, err
	}
	return deviceToken, true, nil
}

// DeviceByToken authenticates a connecting agent.
func (s *Store) DeviceByToken(token string) (Device, error) {
	return s.deviceWhere(`token_hash = ?`, HashToken(token))
}

// DeviceByID looks a device up directly.
func (s *Store) DeviceByID(id string) (Device, error) {
	return s.deviceWhere(`id = ?`, id)
}

func (s *Store) deviceWhere(where string, arg any) (Device, error) {
	var d Device
	var account, name, ver, os, host sql.NullString
	var sim int
	var created, claimed, seen, revoked int64
	err := s.db.QueryRow(`
		SELECT id, account_id, name, agent_version, os, hostname, simulator,
		       sleep_after, created_at, claimed_at, last_seen_at, revoked_at
		  FROM devices WHERE `+where, arg).
		Scan(&d.ID, &account, &name, &ver, &os, &host, &sim, &d.SleepAfter,
			&created, &claimed, &seen, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	d.AccountID, d.Name, d.AgentVersion, d.OS, d.Hostname =
		account.String, name.String, ver.String, os.String, host.String
	d.Simulator = sim == 1
	d.CreatedAt, d.ClaimedAt, d.LastSeenAt, d.RevokedAt =
		fromMs(created), fromMs(claimed), fromMs(seen), fromMs(revoked)
	return d, nil
}

// TouchDevice records what the agent told us about itself, and that it is alive.
func (s *Store) TouchDevice(deviceID, agentVersion, osName, hostname string, simulator bool, sleepAfter int) error {
	sim := 0
	if simulator {
		sim = 1
	}
	_, err := s.db.Exec(`
		UPDATE devices SET agent_version = ?, os = ?, hostname = ?, simulator = ?,
		                   sleep_after = ?, last_seen_at = ?
		 WHERE id = ?`,
		agentVersion, osName, hostname, sim, sleepAfter, ms(s.now().UTC()), deviceID)
	return err
}

// SeenDevice just bumps the heartbeat timestamp.
func (s *Store) SeenDevice(deviceID string) error {
	_, err := s.db.Exec(`UPDATE devices SET last_seen_at = ? WHERE id = ?`, ms(s.now().UTC()), deviceID)
	return err
}

// Devices lists the PCs linked to an account, newest first.
func (s *Store) Devices(accountID string) ([]Device, error) {
	rows, err := s.db.Query(`
		SELECT id FROM devices
		 WHERE account_id = ? AND claimed_at != 0
		 ORDER BY claimed_at DESC`, accountID)
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
	out := make([]Device, 0, len(ids))
	for _, id := range ids {
		d, err := s.DeviceByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// RevokeDevice is the "unlink this PC" button. The agent's token stops working
// immediately, and it can never be un-revoked: pair again to get a new one.
func (s *Store) RevokeDevice(accountID, deviceID string) error {
	res, err := s.db.Exec(
		`UPDATE devices SET revoked_at = ? WHERE id = ? AND account_id = ? AND revoked_at = 0`,
		ms(s.now().UTC()), deviceID, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AllDevices powers the admin view.
func (s *Store) AllDevices() ([]Device, error) {
	rows, err := s.db.Query(`SELECT id FROM devices WHERE claimed_at != 0 ORDER BY claimed_at DESC`)
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
	out := make([]Device, 0, len(ids))
	for _, id := range ids {
		d, err := s.DeviceByID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
