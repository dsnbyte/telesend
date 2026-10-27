// Package testutil provides a fake Telegram Bot API for tests.
package testutil

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/telegram"
)

// Call is one recorded Bot API request.
type Call struct {
	Method string
	Token  string
	JSON   map[string]any    // set for JSON requests
	Fields map[string]string // set for multipart requests
	Files  map[string]string // multipart file field -> "filename:content"
}

// Telegram is a fake Bot API server.
type Telegram struct {
	*httptest.Server
	mu    sync.Mutex
	calls []Call
	// Fail, when set, makes every non-getMe call fail with this description.
	Fail string
}

// Calls returns a copy of the recorded calls (excluding getMe).
func (t *Telegram) Calls() []Call {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Call(nil), t.calls...)
}

// Methods lists the recorded call methods (excluding getMe).
func (t *Telegram) Methods() []string {
	var out []string
	for _, c := range t.Calls() {
		out = append(out, c.Method)
	}
	return out
}

// NewTelegram starts a fake Bot API. Tokens starting with "two" resolve to a
// second bot; every other token to "default_bot".
func NewTelegram(tb testing.TB) *Telegram {
	tb.Helper()
	t := &Telegram{}
	t.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		call := Call{Method: parts[len(parts)-1], Token: strings.TrimPrefix(parts[0], "bot")}
		if call.Method == "getMe" {
			id, name := 1, "default"
			if strings.HasPrefix(call.Token, "two") {
				id, name = 2, "other"
			}
			writeJSON(w, 200, map[string]any{"ok": true, "result": map[string]any{
				"id": id, "is_bot": true, "first_name": name, "username": name + "_bot"}})
			return
		}
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if strings.HasPrefix(mediaType, "multipart/") {
			call.Fields, call.Files = map[string]string{}, map[string]string{}
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(p)
				if p.FileName() != "" {
					call.Files[p.FormName()] = p.FileName() + ":" + string(data)
				} else {
					call.Fields[p.FormName()] = string(data)
				}
			}
		} else {
			dec := json.NewDecoder(r.Body)
			dec.UseNumber()
			_ = dec.Decode(&call.JSON)
		}
		t.mu.Lock()
		t.calls = append(t.calls, call)
		n, fail := len(t.calls), t.Fail
		t.mu.Unlock()
		if fail != "" {
			writeJSON(w, 400, map[string]any{"ok": false, "error_code": 400, "description": fail})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "result": map[string]any{"message_id": n}})
	}))
	tb.Cleanup(t.Close)
	return t
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NewApp returns an in-memory App wired to a fake Telegram.
func NewApp(tb testing.TB) (*app.App, *Telegram) {
	tb.Helper()
	tg := NewTelegram(tb)
	client := telegram.NewClient()
	client.BaseURL = tg.URL
	a, err := app.New(":memory:", client)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { a.Close() })
	return a, tg
}

// WithBot registers the default bot and returns the app.
func WithBot(tb testing.TB) (*app.App, *Telegram) {
	tb.Helper()
	a, tg := NewApp(tb)
	if _, err := a.RegisterBot(context.Background(), "token"); err != nil {
		tb.Fatal(err)
	}
	return a, tg
}

// AppWrapper wraps *app.App to avoid test circular dependencies if needed.
type AppWrapper struct {
	*app.App
}
