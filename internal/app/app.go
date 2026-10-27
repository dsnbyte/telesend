// Package app wires storage and the Telegram client into Telesend's use cases.
// The CLI, REST API, and MCP servers all share it.
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"strconv"
	"strings"

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
	OAuth        *store.OAuth
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
	return &App{
		DB: db, DatabasePath: path,
		Aliases: store.NewAliases(db), Bots: store.NewBots(db), OAuth: store.NewOAuth(db),
		Telegram: client,
	}, nil
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
	return &Delivery{
		Bot: bot.Username, ChatID: rcpt.ChatID, ThreadID: rcpt.ThreadID,
		MessageID: messageID(raw), Result: raw,
	}, nil
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
