package store

import (
	"database/sql"
	"errors"
	"time"
)

const serversSchema = `
-- A server the user has starred. The canonical identity is the search
-- provider's id, not the address, because Rust server IPs change and the
-- address has to be looked up fresh each time we connect.
CREATE TABLE IF NOT EXISTS favourites (
  account_id  TEXT NOT NULL REFERENCES accounts(id),
  server_id   TEXT NOT NULL,
  name        TEXT NOT NULL DEFAULT '',
  address     TEXT NOT NULL DEFAULT '',
  region      TEXT NOT NULL DEFAULT '',
  added_at    INTEGER NOT NULL,
  PRIMARY KEY (account_id, server_id)
);
`

// Favourite is a starred server.
type Favourite struct {
	ServerID string    `json:"server_id"`
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Region   string    `json:"region"`
	AddedAt  time.Time `json:"added_at"`
}

// AddFavourite stars a server, or updates what we know about one already
// starred. The address is only a hint for display: it is looked up again at the
// moment we connect.
func (s *Store) AddFavourite(accountID string, f Favourite) error {
	_, err := s.db.Exec(`
		INSERT INTO favourites (account_id, server_id, name, address, region, added_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, server_id) DO UPDATE SET
			name = excluded.name, address = excluded.address, region = excluded.region`,
		accountID, f.ServerID, f.Name, f.Address, f.Region, ms(s.now().UTC()))
	return err
}

// RemoveFavourite unstars a server.
func (s *Store) RemoveFavourite(accountID, serverID string) error {
	_, err := s.db.Exec(
		`DELETE FROM favourites WHERE account_id = ? AND server_id = ?`, accountID, serverID)
	return err
}

// Favourites lists an account's starred servers, most recently added first.
func (s *Store) Favourites(accountID string) ([]Favourite, error) {
	rows, err := s.db.Query(`
		SELECT server_id, name, address, region, added_at
		  FROM favourites WHERE account_id = ? ORDER BY added_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Favourite{}
	for rows.Next() {
		var f Favourite
		var added int64
		if err := rows.Scan(&f.ServerID, &f.Name, &f.Address, &f.Region, &added); err != nil {
			return nil, err
		}
		f.AddedAt = fromMs(added)
		out = append(out, f)
	}
	return out, rows.Err()
}

// Favourite reads one starred server.
func (s *Store) Favourite(accountID, serverID string) (Favourite, error) {
	var f Favourite
	var added int64
	err := s.db.QueryRow(`
		SELECT server_id, name, address, region, added_at
		  FROM favourites WHERE account_id = ? AND server_id = ?`, accountID, serverID).
		Scan(&f.ServerID, &f.Name, &f.Address, &f.Region, &added)
	if errors.Is(err, sql.ErrNoRows) {
		return Favourite{}, ErrNotFound
	}
	if err != nil {
		return Favourite{}, err
	}
	f.AddedAt = fromMs(added)
	return f, nil
}
