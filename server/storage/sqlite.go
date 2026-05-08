package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"grunt/client"
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
			user TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL DEFAULT ''
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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS invites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_by_user TEXT,
			used_at DATETIME
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create invites table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) CreateUser(user, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.db.Exec("INSERT OR IGNORE INTO users (user, password_hash) VALUES (?, ?)", user, string(hash))
	return err
}

func (s *Store) VerifyUser(user, password string) (bool, error) {
	var hash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE user = ?", user).Scan(&hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateUserCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CreateInvite(code string, expiresAt time.Time, createdBy string) error {
	_, err := s.db.Exec(
		"INSERT INTO invites (code, expires_at, created_by_user) VALUES (?, ?, ?)",
		code, expiresAt.Format(time.RFC3339), createdBy,
	)
	return err
}

func (s *Store) ValidateInvite(code string) (bool, error) {
	var expiresAt string
	err := s.db.QueryRow(
		"SELECT expires_at FROM invites WHERE code = ? AND used_at IS NULL",
		code,
	).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	expTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, err
	}

	return time.Now().Before(expTime), nil
}

func (s *Store) MarkInviteUsed(code string, userId string) error {
	_, err := s.db.Exec(
		"UPDATE invites SET used_at = ?, created_by_user = ? WHERE code = ? AND used_at IS NULL",
		time.Now().Format(time.RFC3339), userId, code,
	)
	return err
}

func (s *Store) Save(msg *client.Broadcast) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO messages (content, client_id, user, timestamp) VALUES (?, ?, ?, ?)",
		msg.Content, msg.ClientID, msg.UserID, msg.Timestamp,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Sync(since int, limit int) ([]client.Broadcast, error) {
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

	var msgs []client.Broadcast
	for rows.Next() {
		var m client.Broadcast
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