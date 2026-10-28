package store

import (
	"database/sql"
	"errors"
	"time"
)

// UIAuth manages SQLite persistence for UI sessions.
type UIAuth struct {
	db *sql.DB
}

// NewUIAuth builds a UIAuth repository.
func NewUIAuth(db *sql.DB) *UIAuth {
	return &UIAuth{db: db}
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
