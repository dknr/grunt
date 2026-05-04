package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"grunt/internal/message"
)

type Store struct {
	db *sql.DB
}

func New() (*Store, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			client_id TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Save(msg *message.Broadcast) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO messages (content, client_id, timestamp) VALUES (?, ?, ?)",
		msg.Content, msg.ClientID, msg.Timestamp,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Sync(since int) ([]message.Broadcast, error) {
	rows, err := s.db.QueryContext(context.Background(),
		"SELECT id, content, client_id, timestamp FROM messages WHERE id > ? ORDER BY id ASC",
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []message.Broadcast
	for rows.Next() {
		var m message.Broadcast
		var ts string
		if err := rows.Scan(&m.ID, &m.Content, &m.ClientID, &ts); err != nil {
			return nil, err
		}
		m.Timestamp, _ = time.Parse(time.RFC3339, ts)
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}