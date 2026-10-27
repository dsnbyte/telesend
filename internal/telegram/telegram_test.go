package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogIntegrity(t *testing.T) {
	if len(Catalog) != 23 {
		t.Fatalf("expected 23 operations, got %d", len(Catalog))
	}
	for _, op := range Catalog {
		if op.Type == "" || op.Method == "" || op.Description == "" {
			t.Fatalf("incomplete operation: %+v", op)
		}
		found, ok := Get(op.Type)
		if !ok || found.Method != op.Method {
			t.Fatalf("Get(%q) failed", op.Type)
		}
	}
}

func TestTelegramClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTEST/getMe" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"Test","username":"test_bot"}}`))
			return
		}
		if r.URL.Path == "/botTEST/sendMessage" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{"message_id":456}}`))
			return
		}
		if r.URL.Path == "/botTEST/sendPhoto" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{"message_id":789}}`))
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer ts.Close()

	c := &Client{HTTP: ts.Client(), BaseURL: ts.URL}
	u, err := c.GetMe(context.Background(), "TEST")
	if err != nil || u.Username != "test_bot" || !u.IsBot {
		t.Fatalf("getMe: %+v %v", u, err)
	}

	raw, err := c.Call(context.Background(), "TEST", "sendMessage", map[string]any{"chat_id": "1", "text": "hello"})
	if err != nil || string(raw) != `{"message_id":456}` {
		t.Fatalf("sendMessage: %s %v", string(raw), err)
	}

	// Test with local file upload
	tmp := filepath.Join(t.TempDir(), "pic.png")
	os.WriteFile(tmp, []byte("fake image data"), 0o644)
	photoRaw, err := c.Call(context.Background(), "TEST", "sendPhoto", map[string]any{
		"chat_id": "1",
		"photo":   map[string]any{"source": "path", "value": tmp},
		"caption": "test",
	})
	if err != nil || string(photoRaw) != `{"message_id":789}` {
		t.Fatalf("sendPhoto: %s %v", string(photoRaw), err)
	}
}
