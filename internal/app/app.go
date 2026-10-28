// Package app wires storage and the Telegram client into Telesend's use cases.
// The CLI, REST API, and MCP servers all share it.
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/telegram"
)

// App is the shared application core.
type App struct {
	DB           *sql.DB
	DatabasePath string
	Aliases      *store.Aliases
	Bots         *store.Bots
	History      *store.History
	OAuth        *store.OAuth
	UIAuth       *store.UIAuth
	Telegram     *telegram.Client
}

// New opens the database at path (":memory:" for tests) and builds an App.
// A nil client uses the public Telegram Bot API.
func New(path string, client *telegram.Client) (*App, error) {
	db, err := store.Open(path)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = telegram.NewClient()
	}
	retentionDays := getRetentionDays()
	return &App{
		DB: db, DatabasePath: path,
		Aliases: store.NewAliases(db), Bots: store.NewBots(db), History: store.NewHistory(db, retentionDays),
		OAuth: store.NewOAuth(db),
		UIAuth: store.NewUIAuth(db),
		Telegram: client,
	}, nil
}

func getRetentionDays() int {
	if s := os.Getenv("TELESEND_HISTORY_RETENTION_DAYS"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	if s := os.Getenv("TELESEND_HISTORY_RETENTION"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	return 7
}

// Close releases the database.
func (a *App) Close() error { return a.DB.Close() }

// RegisterBot verifies token with Telegram and stores the bot.
func (a *App) RegisterBot(ctx context.Context, token string) (store.Bot, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return store.Bot{}, apperr.New(apperr.Validation, "Bot token is required")
	}
	user, err := a.Telegram.GetMe(ctx, token)
	if err != nil {
		return store.Bot{}, err
	}
	if !user.IsBot || user.Username == "" {
		return store.Bot{}, apperr.New(apperr.Validation, "Telegram token did not resolve to a bot with a username")
	}
	return a.Bots.Add(store.VerifiedBot{
		TelegramID: formatID(user.ID),
		Name:       strings.TrimSpace(user.FirstName + " " + user.LastName),
		Username:   user.Username,
		Token:      token,
	})
}

// UpdateBot updates the name and/or token of an existing bot.
// If a new token is supplied, it is verified against Telegram and must match the bot's Telegram ID.
func (a *App) UpdateBot(ctx context.Context, username string, in store.BotInput) (store.Bot, error) {
	existing, err := a.Bots.Find(username)
	if err != nil {
		return store.Bot{}, err
	}
	if existing == nil {
		return store.Bot{}, apperr.Newf(apperr.NotFound, "Bot @%s was not found", strings.TrimPrefix(username, "@"))
	}
	name := strings.TrimSpace(in.Name)
	token := strings.TrimSpace(in.Token)
	if token != "" {
		user, err := a.Telegram.GetMe(ctx, token)
		if err != nil {
			return store.Bot{}, err
		}
		if !user.IsBot || user.Username == "" {
			return store.Bot{}, apperr.New(apperr.Validation, "Telegram token did not resolve to a bot with a username")
		}
		if formatID(user.ID) != existing.TelegramID {
			return store.Bot{}, apperr.New(apperr.Validation, "Token belongs to a different Telegram bot ID")
		}
		if name == "" {
			name = strings.TrimSpace(user.FirstName + " " + user.LastName)
		}
	}
	return a.Bots.Update(username, store.BotInput{Name: name, Token: token})
}

// ResolveBot returns the named bot, or the default bot when username is empty.
func (a *App) ResolveBot(username string) (*store.StoredBot, error) {
	if username != "" {
		bot, err := a.Bots.Find(username)
		if err != nil {
			return nil, err
		}
		if bot == nil {
			return nil, apperr.Newf(apperr.NotFound, "Bot @%s was not found", strings.TrimPrefix(username, "@"))
		}
		return bot, nil
	}
	bot, err := a.Bots.Default()
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, apperr.New(apperr.Validation, "No default bot is configured; register or select a bot first")
	}
	return bot, nil
}

// Recipient is a resolved chat and optional forum topic.
type Recipient struct {
	ChatID   string
	ThreadID int // 0 when not set
}

// ResolveRecipient turns target into a chat. A numeric target is used as a
// chat ID directly; anything else must be an alias. An explicit thread ID
// (non-zero) overrides the alias's stored thread. A thread ID can also be
// specified by appending #<thread_id> to the target. For private chats,
// the thread ID is ignored.
func (a *App) ResolveRecipient(target string, explicitThread int) (Recipient, error) {
	if explicitThread < 0 {
		return Recipient{}, apperr.New(apperr.Validation, "Message thread ID must be a positive integer")
	}
	target = strings.TrimSpace(target)
	base := target
	var inlineThread int
	if strings.Contains(target, "#") {
		parts := strings.Split(target, "#")
		if len(parts) != 2 {
			return Recipient{}, apperr.New(apperr.Validation, "Message thread ID must be a positive integer")
		}
		base = strings.TrimSpace(parts[0])
		threadStr := strings.TrimSpace(parts[1])
		tid, err := strconv.Atoi(threadStr)
		if err != nil || tid <= 0 {
			return Recipient{}, apperr.New(apperr.Validation, "Message thread ID must be a positive integer")
		}
		inlineThread = tid
	}

	var chatID string
	var aliasThread *int
	if store.IsChatID(base) {
		chatID = base
	} else {
		alias, err := a.Aliases.Find(base)
		if err != nil {
			return Recipient{}, err
		}
		if alias == nil {
			return Recipient{}, apperr.Newf(apperr.NotFound, "Recipient alias %q was not found", base)
		}
		chatID = alias.ChatID
		aliasThread = alias.MessageThreadID
	}

	thread := 0
	if explicitThread > 0 {
		thread = explicitThread
	} else if inlineThread > 0 {
		thread = inlineThread
	} else if aliasThread != nil {
		thread = *aliasThread
	}

	if store.TypeOf(chatID) == store.TypePrivate {
		thread = 0
	}

	return Recipient{ChatID: chatID, ThreadID: thread}, nil
}

// SendRequest describes one message delivery.
type SendRequest struct {
	Type     string
	To       string
	Bot      string
	ThreadID int
	Payload  map[string]any
}

// Delivery is the outcome of a successful send.
type Delivery struct {
	Bot       string          `json:"bot"`
	ChatID    string          `json:"chatId"`
	ThreadID  int             `json:"messageThreadId,omitempty"`
	MessageID int64           `json:"messageId,omitempty"`
	Result    json.RawMessage `json:"result"`
}

// Send validates and delivers a message through Telegram.
func (a *App) Send(ctx context.Context, req SendRequest) (*Delivery, error) {
	op, ok := telegram.Get(req.Type)
	if !ok {
		return nil, apperr.Newf(apperr.Validation, "Unsupported message type: %s", req.Type)
	}
	for _, field := range op.Required {
		switch v := req.Payload[field].(type) {
		case nil:
			return nil, apperr.Newf(apperr.Validation, "Field %q is required for %s", field, req.Type)
		case string:
			if v == "" {
				return nil, apperr.Newf(apperr.Validation, "Field %q is required for %s", field, req.Type)
			}
		}
	}
	if err := validateMedia(req.Payload, op.AllowURL, ""); err != nil {
		return nil, err
	}

	bot, err := a.ResolveBot(req.Bot)
	if err != nil {
		return nil, err
	}
	rcpt, err := a.ResolveRecipient(req.To, req.ThreadID)
	if err != nil {
		return nil, err
	}
	payload := maps.Clone(req.Payload)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["chat_id"] = rcpt.ChatID
	if rcpt.ThreadID != 0 {
		payload["message_thread_id"] = rcpt.ThreadID
	} else {
		delete(payload, "message_thread_id")
	}

	raw, err := a.Telegram.Call(ctx, bot.Token, op.Method, payload)
	if err != nil {
		return nil, err
	}
	mid := messageID(raw)
	if a.History != nil {
		var msgIDPtr *int64
		if mid != 0 {
			msgIDPtr = &mid
		}
		var threadPtr *int
		if rcpt.ThreadID != 0 {
			threadPtr = &rcpt.ThreadID
		}
		summary := summarizePayload(req.Type, req.Payload)
		sanitizedPayload := sanitizePayload(req.Payload)
		payloadBytes, _ := json.Marshal(sanitizedPayload)
		_, _ = a.History.Record(store.HistoryEntry{
			Bot:             bot.Username,
			ChatID:          rcpt.ChatID,
			Recipient:       req.To,
			MessageThreadID: threadPtr,
			MessageType:     req.Type,
			Summary:         summary,
			Payload:         payloadBytes,
			MessageID:       msgIDPtr,
			Status:          "delivered",
		})
	}
	return &Delivery{
		Bot: bot.Username, ChatID: rcpt.ChatID, ThreadID: rcpt.ThreadID,
		MessageID: mid, Result: raw,
	}, nil
}

func summarizePayload(msgType string, payload map[string]any) string {
	if payload == nil {
		return msgType
	}
	if text, ok := payload["text"].(string); ok && strings.TrimSpace(text) != "" {
		s := strings.TrimSpace(text)
		if len(s) > 120 {
			return s[:117] + "..."
		}
		return s
	}
	if caption, ok := payload["caption"].(string); ok && strings.TrimSpace(caption) != "" {
		s := strings.TrimSpace(caption)
		if len(s) > 120 {
			return s[:117] + "..."
		}
		return s
	}
	if msgType == "poll" {
		if q, ok := payload["question"].(string); ok && strings.TrimSpace(q) != "" {
			return "Poll: " + strings.TrimSpace(q)
		}
	}
	if msgType == "dice" {
		if emoji, ok := payload["emoji"].(string); ok && emoji != "" {
			return "Dice: " + emoji
		}
		return "Dice: 🎲"
	}
	if msgType == "location" {
		if lat, ok1 := payload["latitude"]; ok1 {
			if lon, ok2 := payload["longitude"]; ok2 {
				return fmt.Sprintf("Location: %v, %v", lat, lon)
			}
		}
	}
	if op, ok := telegram.Get(msgType); ok && len(op.MediaFields) > 0 {
		field := op.MediaFields[0]
		if media, ok := payload[field].(map[string]any); ok {
			src, _ := media["source"].(string)
			val, _ := media["value"].(string)
			if src != "" && val != "" {
				return fmt.Sprintf("%s (%s: %s)", msgType, src, val)
			}
			if u, ok := media["file"].(telegram.Upload); ok {
				return fmt.Sprintf("%s (upload: %s)", msgType, u.Name())
			}
		}
	}
	return msgType
}

func sanitizePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(payload))
	for k, v := range payload {
		if k == "chat_id" || k == "message_thread_id" {
			continue
		}
		out[k] = sanitizeValue(v)
	}
	return out
}

func sanitizeValue(v any) any {
	switch val := v.(type) {
	case telegram.Upload:
		return map[string]any{"filename": val.Name()}
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, child := range val {
			res[k] = sanitizeValue(child)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, child := range val {
			res[i] = sanitizeValue(child)
		}
		return res
	default:
		return val
	}
}

func messageID(raw json.RawMessage) int64 {
	var one struct {
		MessageID int64 `json:"message_id"`
	}
	if json.Unmarshal(raw, &one) == nil && one.MessageID != 0 {
		return one.MessageID
	}
	var many []struct {
		MessageID int64 `json:"message_id"`
	}
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return many[0].MessageID
	}
	return 0
}

func formatID(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}

func validateMedia(v any, allowURL bool, key string) error {
	switch t := v.(type) {
	case map[string]any:
		src, ok := telegram.MediaSourceOf(t)
		if !ok {
			for k, item := range t {
				if err := validateMedia(item, allowURL, k); err != nil {
					return err
				}
			}
			return nil
		}
		value, _ := t["value"].(string)
		switch {
		case src == "url" && !allowURL:
			return apperr.New(apperr.Validation, "URL media is not supported for this message type")
		case key == "thumbnail" && src != "path" && src != "upload":
			return apperr.New(apperr.Validation, "Thumbnail must be a new uploaded file")
		case src != "upload" && value == "":
			return apperr.Newf(apperr.Validation, "Media source %s requires a value", src)
		case src == "upload":
			if _, ok := t["file"].(telegram.Upload); !ok {
				return apperr.New(apperr.Validation, "Uploaded media is missing file content")
			}
		}
	case []any:
		for _, item := range t {
			if err := validateMedia(item, allowURL, key); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteHistoryResult holds the outcome of deleting a history record.
type DeleteHistoryResult struct {
	ID              int64 `json:"id"`
	Deleted         bool  `json:"deleted"`
	TelegramDeleted bool  `json:"telegramDeleted,omitempty"`
}

// DeleteHistory removes a history record by ID, optionally deleting the message from Telegram as well.
func (a *App) DeleteHistory(ctx context.Context, id int64, deleteTelegram bool) (*DeleteHistoryResult, error) {
	entry, err := a.History.Get(id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, apperr.Newf(apperr.NotFound, "History entry %d was not found", id)
	}

	if deleteTelegram {
		if entry.MessageID == nil || *entry.MessageID == 0 {
			return nil, apperr.New(apperr.Validation, "Cannot delete from Telegram: no message ID recorded for this entry. It can only be deleted from history.")
		}
		bot, err := a.Bots.Find(entry.Bot)
		if err != nil {
			return nil, err
		}
		if bot == nil || bot.Token == "" {
			return nil, apperr.Newf(apperr.Validation, "Cannot delete from Telegram: bot @%s is not configured. It can only be deleted from history.", entry.Bot)
		}

		if err := a.Telegram.DeleteMessage(ctx, bot.Token, entry.ChatID, *entry.MessageID); err != nil {
			isOld := false
			if t, parseErr := time.Parse("2006-01-02T15:04:05.000Z", entry.CreatedAt); parseErr == nil {
				if time.Since(t) > 48*time.Hour {
					isOld = true
				}
			} else if t, parseErr := time.Parse(time.RFC3339Nano, entry.CreatedAt); parseErr == nil {
				if time.Since(t) > 48*time.Hour {
					isOld = true
				}
			}

			errStr := strings.ToLower(err.Error())
			if isOld || strings.Contains(errStr, "can't be deleted") || strings.Contains(errStr, "cant be deleted") || strings.Contains(errStr, "not found") {
				return nil, apperr.New(apperr.Validation, "Message is older than 48 hours and cannot be deleted from Telegram. It can only be deleted from history.")
			}
			return nil, apperr.Newf(apperr.Telegram, "Failed to delete from Telegram: %s. It can only be deleted from history.", apperr.Redact(err.Error(), bot.Token))
		}
	}

	deleted, err := a.History.Delete(id)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, apperr.Newf(apperr.NotFound, "History entry %d was not found", id)
	}

	return &DeleteHistoryResult{
		ID:              id,
		Deleted:         true,
		TelegramDeleted: deleteTelegram,
	}, nil
}

// ExportDatabase dumps the database schema and data as an SQL script.
func (a *App) ExportDatabase() (string, error) {
	return store.ExportSQL(a.DB)
}

// ImportDatabase replaces the database content with the provided SQL script.
func (a *App) ImportDatabase(script string) error {
	return store.ImportSQL(a.DB, script)
}

