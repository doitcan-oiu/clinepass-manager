package store

import "opencode-go-manager/internal/model"

func (s *Store) migrateAutoConnection() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS auto_connection (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		url TEXT NOT NULL DEFAULT '',
		token TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// GetAutoConnection returns sql.ErrNoRows until a connection has been saved;
// callers can then use the process configuration as a bootstrap default.
func (s *Store) GetAutoConnection() (model.AutoConnection, error) {
	var c model.AutoConnection
	err := s.db.QueryRow(`SELECT url, token FROM auto_connection WHERE id = 1`).Scan(&c.URL, &c.Token)
	return c, err
}

func (s *Store) SaveAutoConnection(c model.AutoConnection) error {
	_, err := s.db.Exec(`INSERT INTO auto_connection (id, url, token) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET url = excluded.url, token = excluded.token`, c.URL, c.Token)
	return err
}
