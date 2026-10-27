package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/dsnbyte/telesend/internal/apperr"
)

// Bot is the safe, token-free projection of a registered bot.
type Bot struct {
	TelegramID string `json:"telegramId"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	IsDefault  bool   `json:"isDefault"`
}

// StoredBot includes the secret token. Never serialize it.
type StoredBot struct {
	Bot
	Token string
}

// VerifiedBot is a bot whose identity Telegram has confirmed.
type VerifiedBot struct {
	TelegramID, Name, Username, Token string
}

// Bots persists registered bots.
type Bots struct{ db *sql.DB }

// NewBots wraps db.
func NewBots(db *sql.DB) *Bots { return &Bots{db} }

func normalizeUsername(u string) string { return strings.TrimPrefix(strings.TrimSpace(u), "@") }

const botColumns = "telegram_id, name, username, token, is_default"

func scanBot(row interface{ Scan(...any) error }) (*StoredBot, error) {
	var b StoredBot
	var def int
	err := row.Scan(&b.TelegramID, &b.Name, &b.Username, &b.Token, &def)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.IsDefault = def == 1
	return &b, nil
}

func findBot(q queryer, username string) (*StoredBot, error) {
	return scanBot(q.QueryRow("SELECT "+botColumns+" FROM bots WHERE username = ? COLLATE NOCASE", normalizeUsername(username)))
}

func requireBot(q queryer, username string) (*StoredBot, error) {
	b, err := findBot(q, username)
	if err == nil && b == nil {
		return nil, apperr.Newf(apperr.NotFound, "Bot @%s was not found", normalizeUsername(username))
	}
	return b, err
}

// Add stores a bot. The first bot becomes the default.
func (r *Bots) Add(bot VerifiedBot) (Bot, error) {
	name, username := strings.TrimSpace(bot.Name), normalizeUsername(bot.Username)
	if bot.TelegramID == "" || name == "" || username == "" || strings.TrimSpace(bot.Token) == "" {
		return Bot{}, apperr.New(apperr.Validation, "Telegram bot identity is incomplete")
	}
	var out Bot
	err := inTx(r.db, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM bots").Scan(&count); err != nil {
			return err
		}
		isDefault := 0
		if count == 0 {
			isDefault = 1
		}
		ts := now()
		if _, err := tx.Exec(
			`INSERT INTO bots (telegram_id, name, username, token, is_default, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			bot.TelegramID, name, username, bot.Token, isDefault, ts, ts); err != nil {
			return err
		}
		stored, err := requireBot(tx, username)
		if err != nil {
			return err
		}
		out = stored.Bot
		return nil
	})
	if isUnique(err) {
		return Bot{}, apperr.New(apperr.Conflict, "This Telegram bot is already registered")
	}
	return out, err
}

// List returns all bots, default first.
func (r *Bots) List() ([]Bot, error) {
	rows, err := r.db.Query("SELECT " + botColumns + " FROM bots ORDER BY is_default DESC, username COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bots := []Bot{}
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		bots = append(bots, b.Bot)
	}
	return bots, rows.Err()
}

// Find returns the bot with username, or nil.
func (r *Bots) Find(username string) (*StoredBot, error) { return findBot(r.db, username) }

// Default returns the default bot, or nil.
func (r *Bots) Default() (*StoredBot, error) {
	return scanBot(r.db.QueryRow("SELECT " + botColumns + " FROM bots WHERE is_default = 1"))
}

// SetDefault makes username the only default bot.
func (r *Bots) SetDefault(username string) (Bot, error) {
	var out Bot
	err := inTx(r.db, func(tx *sql.Tx) error {
		bot, err := requireBot(tx, username)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE bots SET is_default = 0 WHERE is_default = 1"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE bots SET is_default = 1, updated_at = ? WHERE username = ?", now(), bot.Username); err != nil {
			return err
		}
		out = bot.Bot
		out.IsDefault = true
		return nil
	})
	return out, err
}

// Remove deletes a bot. The default cannot be removed while others remain.
func (r *Bots) Remove(username string) (Bot, error) {
	var out Bot
	err := inTx(r.db, func(tx *sql.Tx) error {
		bot, err := requireBot(tx, username)
		if err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM bots").Scan(&count); err != nil {
			return err
		}
		if bot.IsDefault && count > 1 {
			return apperr.New(apperr.Conflict, "Select another default bot before removing the current default")
		}
		if _, err := tx.Exec("DELETE FROM bots WHERE username = ? COLLATE NOCASE", bot.Username); err != nil {
			return err
		}
		out = bot.Bot
		return nil
	})
	return out, err
}
