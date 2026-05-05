package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"

	"grunt/internal/message"
)

type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	slog.Info("Database loaded", "dsn", dsn)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			user TEXT PRIMARY KEY
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create users table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			client_id TEXT,
			user TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create messages table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) CreateUser(user string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO users (user) VALUES (?)", user)
	return err
}

func (s *Store) UserExists(user string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE user = ?", user).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Save(msg *message.Broadcast) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO messages (content, client_id, user, timestamp) VALUES (?, ?, ?, ?)",
		msg.Content, msg.ClientID, msg.UserID, msg.Timestamp,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Sync(since int, limit int) ([]message.Broadcast, error) {
	var query string
	var args []interface{}

	if limit > 0 {
		query = "SELECT id, content, client_id, user, timestamp FROM messages WHERE id > ? ORDER BY id DESC LIMIT ?"
		args = []interface{}{since, limit}
	} else {
		query = "SELECT id, content, client_id, user, timestamp FROM messages WHERE id > ? ORDER BY id ASC"
		args = []interface{}{since}
	}

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []message.Broadcast
	for rows.Next() {
		var m message.Broadcast
		var ts string
		if err := rows.Scan(&m.ID, &m.Content, &m.ClientID, &m.UserID, &ts); err != nil {
			return nil, err
		}
		m.Timestamp, _ = time.Parse(time.RFC3339, ts)
		msgs = append(msgs, m)
	}

	if limit > 0 {
		// Reverse to return oldest-first
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, rows.Err()
}