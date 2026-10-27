package store

import (
	"database/sql"
	"errors"
	"time"
)

// UIAuth manages SQLite persistence for UI authentication and sessions.
type UIAuth struct {
	db *sql.DB
}

// NewUIAuth builds a UIAuth repository.
func NewUIAuth(db *sql.DB) *UIAuth {
	return &UIAuth{db: db}
}

// HasPassword reports whether an admin UI password has been set.
func (u *UIAuth) HasPassword() (bool, error) {
	var count int
	err := u.db.QueryRow("SELECT COUNT(*) FROM ui_auth WHERE id = 1").Scan(&count)
	return count > 0, err
}

// GetPasswordHash returns the stored password hash, or empty string if not configured.
func (u *UIAuth) GetPasswordHash() (string, error) {
	var hash string
	err := u.db.QueryRow("SELECT password_hash FROM ui_auth WHERE id = 1").Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

// SetPassword stores or updates the password hash in ui_auth.
func (u *UIAuth) SetPassword(hash string) error {
	ts := now()
	_, err := u.db.Exec(`
		INSERT INTO ui_auth (id, password_hash, created_at, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at
	`, hash, ts, ts)
	return err
}

// CreateSession persists a session token with an expiry timestamp.
func (u *UIAuth) CreateSession(token string, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl).Unix()
	ts := now()
	_, err := u.db.Exec("INSERT INTO ui_sessions (token, expires_at, created_at) VALUES (?, ?, ?)",
		token, expiresAt, ts)
	return err
}

// ValidateSession reports whether token exists and has not expired.
func (u *UIAuth) ValidateSession(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var expiresAt int64
	err := u.db.QueryRow("SELECT expires_at FROM ui_sessions WHERE token = ?", token).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	nowUnix := time.Now().Unix()
	if expiresAt <= nowUnix {
		_, _ = u.db.Exec("DELETE FROM ui_sessions WHERE token = ?", token)
		return false, nil
	}
	return true, nil
}

// DeleteSession removes a session token.
func (u *UIAuth) DeleteSession(token string) error {
	_, err := u.db.Exec("DELETE FROM ui_sessions WHERE token = ?", token)
	return err
}

// DeleteAllSessions removes all active sessions.
func (u *UIAuth) DeleteAllSessions() error {
	_, err := u.db.Exec("DELETE FROM ui_sessions")
	return err
}

// CleanupExpiredSessions prunes expired tokens.
func (u *UIAuth) CleanupExpiredSessions() error {
	_, err := u.db.Exec("DELETE FROM ui_sessions WHERE expires_at <= ?", time.Now().Unix())
	return err
}
