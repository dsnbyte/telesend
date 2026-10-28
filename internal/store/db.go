// Package store persists bots, aliases, and OAuth state in SQLite.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/internal/paths"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Migrations are append-only. Never edit an existing entry.
var migrations = []string{
	`
    CREATE TABLE bots (
      id INTEGER PRIMARY KEY,
      telegram_id TEXT NOT NULL UNIQUE,
      name TEXT NOT NULL,
      username TEXT NOT NULL COLLATE NOCASE UNIQUE,
      token TEXT NOT NULL,
      is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
    CREATE UNIQUE INDEX bots_one_default ON bots(is_default) WHERE is_default = 1;

    CREATE TABLE aliases (
      id INTEGER PRIMARY KEY,
      name TEXT NOT NULL COLLATE NOCASE UNIQUE,
      chat_id TEXT NOT NULL,
      message_thread_id INTEGER NULL CHECK (message_thread_id IS NULL OR message_thread_id > 0),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
  `,
	`
    CREATE TABLE oauth_clients (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      redirect_uris TEXT NOT NULL,
      created_at INTEGER NOT NULL
    );

    CREATE TABLE oauth_grants (
      id TEXT PRIMARY KEY,
      client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
      scopes TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      revoked_at INTEGER NULL
    );

    CREATE TABLE oauth_authorization_codes (
      digest TEXT PRIMARY KEY,
      grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
      redirect_uri TEXT NOT NULL,
      code_challenge TEXT NOT NULL,
      expires_at INTEGER NOT NULL,
      consumed_at INTEGER NULL
    );
    CREATE INDEX oauth_authorization_codes_expiry ON oauth_authorization_codes(expires_at);

    CREATE TABLE oauth_access_tokens (
      digest TEXT PRIMARY KEY,
      grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
      expires_at INTEGER NOT NULL,
      revoked_at INTEGER NULL
    );
    CREATE INDEX oauth_access_tokens_expiry ON oauth_access_tokens(expires_at);

    CREATE TABLE oauth_refresh_tokens (
      digest TEXT PRIMARY KEY,
      grant_id TEXT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
      expires_at INTEGER NOT NULL,
      revoked_at INTEGER NULL,
      replaced_by_digest TEXT NULL
    );
    CREATE INDEX oauth_refresh_tokens_expiry ON oauth_refresh_tokens(expires_at);
  `,
	`
    CREATE TABLE ui_auth (
      id INTEGER PRIMARY KEY CHECK (id = 1),
      password_hash TEXT NOT NULL,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );

    CREATE TABLE ui_sessions (
      token TEXT PRIMARY KEY,
      expires_at INTEGER NOT NULL,
      created_at TEXT NOT NULL
    );
    CREATE INDEX ui_sessions_expiry ON ui_sessions(expires_at);
  `,
	`
    CREATE TABLE message_history (
      id INTEGER PRIMARY KEY,
      bot TEXT NOT NULL,
      chat_id TEXT NOT NULL,
      recipient TEXT NOT NULL,
      message_thread_id INTEGER NULL,
      message_type TEXT NOT NULL,
      summary TEXT NOT NULL,
      payload TEXT NOT NULL,
      message_id INTEGER NULL,
      status TEXT NOT NULL,
      created_at TEXT NOT NULL
    );
    CREATE INDEX message_history_created_at ON message_history(created_at);
  `,
}

// Open opens (creating if needed) the database at path. Use ":memory:" for an
// in-memory database. File databases are created with mode 0600 inside a 0700
// directory.
func Open(path string) (*sql.DB, error) {
	dsn := "file::memory:?"
	memory := path == ":memory:"
	if !memory {
		if err := paths.EnsurePrivateDir(filepath.Dir(path)); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
		dsn = "file:" + (&url.URL{Path: path}).EscapedPath() + "?_pragma=journal_mode(WAL)&"
	}
	dsn += "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection keeps in-memory databases coherent and serializes writes.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	if !memory && runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

// Migrate applies pending migrations.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
      version INTEGER PRIMARY KEY,
      applied_at TEXT NOT NULL
    )`); err != nil {
		return err
	}
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for i, script := range migrations {
		version := i + 1
		if applied[version] {
			continue
		}
		err := inTx(db, func(tx *sql.Tx) error {
			if _, err := tx.Exec(script); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", version, now())
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %d: %w", version, err)
		}
	}
	return nil
}

func inTx(db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}
