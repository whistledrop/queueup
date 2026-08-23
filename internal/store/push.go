package store

// PushSubscription is one browser that asked to be notified. The endpoint and
// keys are exactly what the browser's push service handed over.
type PushSubscription struct {
	Endpoint  string `json:"endpoint"`
	AccountID string `json:"-"`
	P256dh    string `json:"p256dh"`
	Auth      string `json:"auth"`
}

// SavePushSubscription registers (or re-registers) a browser.
func (s *Store) SavePushSubscription(accountID string, sub PushSubscription) error {
	_, err := s.db.Exec(`
		INSERT INTO push_subscriptions (endpoint, account_id, p256dh, auth, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			account_id = excluded.account_id, p256dh = excluded.p256dh, auth = excluded.auth`,
		sub.Endpoint, accountID, sub.P256dh, sub.Auth, ms(s.now().UTC()))
	return err
}

// RemovePushSubscription forgets a browser, either because the user turned
// notifications off or because the push service said the subscription is dead.
func (s *Store) RemovePushSubscription(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

// PushSubscriptions lists every browser an account wants notified.
func (s *Store) PushSubscriptions(accountID string) ([]PushSubscription, error) {
	rows, err := s.db.Query(
		`SELECT endpoint, account_id, p256dh, auth FROM push_subscriptions WHERE account_id = ?`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushSubscription{}
	for rows.Next() {
		var p PushSubscription
		if err := rows.Scan(&p.Endpoint, &p.AccountID, &p.P256dh, &p.Auth); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
