package store

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/dsnbyte/telesend/internal/apperr"
)

// Alias types derived from the chat ID.
const (
	TypeGroup   = "group"
	TypePrivate = "private"
)

// Alias maps a short name to a chat and optional forum topic.
type Alias struct {
	Name            string `json:"name"`
	ChatID          string `json:"chatId"`
	MessageThreadID *int   `json:"messageThreadId"`
	Type            string `json:"type"`
}

// AliasInput is the writable part of an Alias.
type AliasInput struct {
	Name            string
	ChatID          string
	MessageThreadID *int
}

var (
	aliasPattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._ -]*[A-Za-z0-9._-])?$`)
	chatIDPattern = regexp.MustCompile(`^-?\d+$`)
	spaces        = regexp.MustCompile(` +`)
)

// IsChatID reports whether s is a signed decimal chat ID.
func IsChatID(s string) bool { return chatIDPattern.MatchString(s) }

// TypeOf classifies a chat ID: negative IDs are groups (including
// supergroups and channels), everything else is private.
func TypeOf(chatID string) string {
	if strings.HasPrefix(chatID, "-") {
		return TypeGroup
	}
	return TypePrivate
}

func normalizeAlias(name string) string { return spaces.ReplaceAllString(strings.TrimSpace(name), " ") }

func validateAlias(in AliasInput) (Alias, error) {
	a := Alias{Name: normalizeAlias(in.Name), ChatID: strings.TrimSpace(in.ChatID), MessageThreadID: in.MessageThreadID}
	if !aliasPattern.MatchString(a.Name) {
		return a, apperr.New(apperr.Validation, "Alias must contain only letters, numbers, spaces, dot, dash, or underscore")
	}
	if !IsChatID(a.ChatID) {
		return a, apperr.New(apperr.Validation, "Chat ID must be a signed decimal integer")
	}
	if a.MessageThreadID != nil && *a.MessageThreadID <= 0 {
		return a, apperr.New(apperr.Validation, "Message thread ID must be a positive integer")
	}
	a.Type = TypeOf(a.ChatID)
	return a, nil
}

// Aliases persists recipient aliases.
type Aliases struct{ db *sql.DB }

// NewAliases wraps db.
func NewAliases(db *sql.DB) *Aliases { return &Aliases{db} }

func scanAlias(row interface{ Scan(...any) error }) (*Alias, error) {
	var a Alias
	var thread sql.NullInt64
	err := row.Scan(&a.Name, &a.ChatID, &thread)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if thread.Valid {
		v := int(thread.Int64)
		a.MessageThreadID = &v
	}
	a.Type = TypeOf(a.ChatID)
	return &a, nil
}

func threadArg(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// Create adds an alias.
func (r *Aliases) Create(in AliasInput) (Alias, error) {
	a, err := validateAlias(in)
	if err != nil {
		return a, err
	}
	ts := now()
	_, err = r.db.Exec(
		"INSERT INTO aliases (name, chat_id, message_thread_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		a.Name, a.ChatID, threadArg(a.MessageThreadID), ts, ts)
	if isUnique(err) {
		return a, apperr.Newf(apperr.Conflict, "Alias %q already exists", a.Name)
	}
	return a, err
}

// List returns all aliases ordered by name.
func (r *Aliases) List() ([]Alias, error) {
	rows, err := r.db.Query("SELECT name, chat_id, message_thread_id FROM aliases ORDER BY name COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Find returns the alias called name (case-insensitive), or nil.
func (r *Aliases) Find(name string) (*Alias, error) {
	return scanAlias(r.db.QueryRow(
		"SELECT name, chat_id, message_thread_id FROM aliases WHERE name = ? COLLATE NOCASE", normalizeAlias(name)))
}

func (r *Aliases) require(name string) (*Alias, error) {
	a, err := r.Find(name)
	if err == nil && a == nil {
		return nil, apperr.Newf(apperr.NotFound, "Alias %q was not found", name)
	}
	return a, err
}

// Update replaces the alias called current.
func (r *Aliases) Update(current string, in AliasInput) (Alias, error) {
	existing, err := r.require(current)
	if err != nil {
		return Alias{}, err
	}
	a, err := validateAlias(in)
	if err != nil {
		return a, err
	}
	_, err = r.db.Exec(
		`UPDATE aliases SET name = ?, chat_id = ?, message_thread_id = ?, updated_at = ?
		 WHERE name = ? COLLATE NOCASE`,
		a.Name, a.ChatID, threadArg(a.MessageThreadID), now(), existing.Name)
	if isUnique(err) {
		return a, apperr.Newf(apperr.Conflict, "Alias %q already exists", a.Name)
	}
	return a, err
}

// Remove deletes the alias called name and returns it.
func (r *Aliases) Remove(name string) (Alias, error) {
	existing, err := r.require(name)
	if err != nil {
		return Alias{}, err
	}
	_, err = r.db.Exec("DELETE FROM aliases WHERE name = ? COLLATE NOCASE", existing.Name)
	return *existing, err
}
