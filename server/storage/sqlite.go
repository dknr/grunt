package storage

import (
	"context"
	"crypto/sha256"
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
			password_hash TEXT DEFAULT NULL,
			is_admin BOOLEAN DEFAULT 0
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create users table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			admin_user_id TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(user),
			FOREIGN KEY (admin_user_id) REFERENCES users(user)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create api_keys table: %w", err)
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
	var hash *string
	if password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		str := string(h)
		hash = &str
	}
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO users (user, password_hash) VALUES (?, ?)",
		user, hash,
	)
	return err
}

func (s *Store) CreateUserNoPassword(user string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO users (user, password_hash) VALUES (?, NULL)",
		user,
	)
	return err
}

func (s *Store) SetUserAdmin(userID string) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = 1 WHERE user = ?", userID)
	return err
}

func (s *Store) IsUserAdmin(userID string) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow("SELECT is_admin FROM users WHERE user = ?", userID).Scan(&isAdmin)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return isAdmin, err
}

func (s *Store) VerifyUser(user, password string) (bool, error) {
	var hash *string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE user = ?", user).Scan(&hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if hash == nil {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(*hash), []byte(password)) == nil, nil
}

func (s *Store) ChangePassword(userID, currentPassword, newPassword string) error {
	// Verify current password first
	ok, err := s.VerifyUser(userID, currentPassword)
	if err != nil {
		return fmt.Errorf("verify current password: %w", err)
	}
	if !ok {
		return fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	// Update password hash in database
	_, err = s.db.Exec("UPDATE users SET password_hash = ? WHERE user = ?", string(hash), userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	return nil
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
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, rows.Err()
}

// API Key functions

func (s *Store) CreateAPIKey(userID, adminUserID, keyHash, name string) error {
	_, err := s.db.Exec(
		"INSERT INTO api_keys (user_id, admin_user_id, key_hash, name) VALUES (?, ?, ?, ?)",
		userID, adminUserID, keyHash, name,
	)
	return err
}

func (s *Store) GetAPIKeyByHash(keyHash string) (string, error) {
	var userID string
	err := s.db.QueryRow("SELECT user_id FROM api_keys WHERE key_hash = ?", keyHash).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("key not found")
	}
	return userID, err
}

func (s *Store) ListAPIKeys(userID string) ([]APIKeyInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, name, created_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyInfo
	for rows.Next() {
		var k APIKeyInfo
		var ts string
		if err := rows.Scan(&k.ID, &k.Name, &ts); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) ListAllAPIKeys() ([]APIKeyInfoFull, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, name, created_at FROM api_keys ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyInfoFull
	for rows.Next() {
		var k APIKeyInfoFull
		var ts string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &ts); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(keyID int64) error {
	_, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", keyID)
	return err
}

// HashAPIKey computes the SHA-256 hash of an API key string.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

type APIKeyInfo struct {
	ID        int64     `json:"id"`
	Name      *string   `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKeyInfoFull struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Name      *string   `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
