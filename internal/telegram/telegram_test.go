package telegram

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		if r.URL.Path == "/botTEST/deleteMessage" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":true}`))
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

	if err := c.DeleteMessage(context.Background(), "TEST", "1", 456); err != nil {
		t.Fatalf("deleteMessage: %v", err)
	}
}

func TestMediaFetcherSSRF(t *testing.T) {
	mf := NewMediaFetcher(false)
	ctx := context.Background()

	blockedURLs := []string{
		"http://127.0.0.1:8080/image.png",
		"http://localhost:8080/image.png",
		"http://10.0.0.1/pic.jpg",
		"http://172.16.0.1/pic.jpg",
		"http://192.168.1.1/pic.jpg",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/pic.jpg",
		"file:///etc/passwd",
		"ftp://example.com/foo.png",
		"://invalid-url",
	}

	for _, u := range blockedURLs {
		_, _, _, err := mf.Fetch(ctx, u, 1024*1024)
		if err == nil {
			t.Errorf("expected SSRF error for %q, got nil", u)
		}
	}
}

func TestMediaFetcherSizeLimit(t *testing.T) {
	largeData := bytes.Repeat([]byte("A"), 1024*100) // 100 KB
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "102400")
		w.Write(largeData)
	}))
	defer mediaServer.Close()

	mf := NewMediaFetcher(true)
	ctx := context.Background()

	// 1. Should fail when Content-Length exceeds maxSize (limit 50 KB)
	_, _, _, err := mf.Fetch(ctx, mediaServer.URL+"/test.png", 50*1024)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Fatalf("expected size limit error, got %v", err)
	}

	// 2. Should succeed when within maxSize (limit 200 KB)
	data, filename, ct, err := mf.Fetch(ctx, mediaServer.URL+"/test.png", 200*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != len(largeData) {
		t.Fatalf("expected %d bytes, got %d", len(largeData), len(data))
	}
	if filename != "test.png" {
		t.Fatalf("expected test.png, got %q", filename)
	}
	if !strings.HasPrefix(ct, "image/png") {
		t.Fatalf("expected image/png, got %q", ct)
	}
}

func TestMediaFetcherFilenameInference(t *testing.T) {
	mf := NewMediaFetcher(true)
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/with-cd":
			w.Header().Set("Content-Disposition", `attachment; filename="custom-avatar.jpg"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("img"))
		case "/by-mime":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("img"))
		case "/fallback":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("img"))
		}
	}))
	defer ts.Close()

	_, fn1, _, err := mf.Fetch(ctx, ts.URL+"/with-cd", 1024)
	if err != nil || fn1 != "custom-avatar.jpg" {
		t.Fatalf("fn1: expected custom-avatar.jpg, got %q %v", fn1, err)
	}

	_, fn2, _, err := mf.Fetch(ctx, ts.URL+"/by-mime", 1024)
	if err != nil || fn2 != "media.png" {
		t.Fatalf("fn2: expected media.png, got %q %v", fn2, err)
	}

	_, fn3, _, err := mf.Fetch(ctx, ts.URL+"/fallback", 1024)
	if err != nil || fn3 != "media.bin" {
		t.Fatalf("fn3: expected media.bin, got %q %v", fn3, err)
	}
}

func TestTelegramClientMediaURLRelay(t *testing.T) {
	imgBytes := []byte("fake-png-image-content")
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(imgBytes)
	}))
	defer imageServer.Close()

	var receivedPhotoField string
	var receivedFileName string
	var receivedFileContent []byte

	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTEST/sendPhoto" {
			mr, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("expected multipart request: %v", err)
			}
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("next part: %v", err)
				}
				switch part.FormName() {
				case "photo":
					buf := new(bytes.Buffer)
					buf.ReadFrom(part)
					receivedPhotoField = buf.String()
				case "attachment_0":
					receivedFileName = part.FileName()
					buf := new(bytes.Buffer)
					buf.ReadFrom(part)
					receivedFileContent = buf.Bytes()
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{"message_id":999}}`))
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer tgServer.Close()

	c := &Client{
		HTTP:         tgServer.Client(),
		BaseURL:      tgServer.URL,
		MediaFetcher: NewMediaFetcher(true), // allow local server
	}

	raw, err := c.Call(context.Background(), "TEST", "sendPhoto", map[string]any{
		"chat_id": "123",
		"photo": map[string]any{
			"source": "url",
			"value":  imageServer.URL + "/generated-photo",
		},
		"caption": "AI generated image",
	})
	if err != nil {
		t.Fatalf("call sendPhoto failed: %v", err)
	}
	if !strings.Contains(string(raw), "999") {
		t.Fatalf("expected message id 999, got %s", string(raw))
	}

	if receivedPhotoField != "attach://attachment_0" {
		t.Fatalf("expected photo field attach://attachment_0, got %q", receivedPhotoField)
	}
	if receivedFileName != "media.png" {
		t.Fatalf("expected attachment filename media.png, got %q", receivedFileName)
	}
	if !bytes.Equal(receivedFileContent, imgBytes) {
		t.Fatalf("expected file bytes %q, got %q", string(imgBytes), string(receivedFileContent))
	}
}
