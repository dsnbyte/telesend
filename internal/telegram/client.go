package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/jsonx"
)

// Client calls the Telegram Bot API.
type Client struct {
	HTTP         *http.Client
	BaseURL      string
	MediaFetcher *MediaFetcher
}

// NewClient returns a client for the public Bot API.
func NewClient() *Client {
	return &Client{
		HTTP:         &http.Client{Timeout: 5 * time.Minute},
		BaseURL:      "https://api.telegram.org",
		MediaFetcher: NewMediaFetcher(false),
	}
}

// User is the subset of the Telegram User object Telesend needs.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// GetMe verifies a token and returns the bot identity.
func (c *Client) GetMe(ctx context.Context, token string) (User, error) {
	raw, err := c.Call(ctx, token, "getMe", nil)
	if err != nil {
		return User{}, err
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return User{}, apperr.New(apperr.Telegram, "Telegram returned an invalid response for getMe")
	}
	return u, nil
}

// DeleteMessage deletes a message from Telegram.
func (c *Client) DeleteMessage(ctx context.Context, token, chatID string, messageID int64) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	_, err := c.Call(ctx, token, "deleteMessage", payload)
	return err
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// Call invokes a Bot API method. Media sources inside payload are resolved:
// file_id values are inlined; url, path and upload values are sent as
// multipart attachments.
func (c *Client) Call(ctx context.Context, token, method string, payload map[string]any) (json.RawMessage, error) {
	fetcher := c.MediaFetcher
	if fetcher == nil {
		fetcher = NewMediaFetcher(false)
	}
	m := &materializer{ctx: ctx, fetcher: fetcher, method: method}
	defer m.close()
	body, err := m.walk(payload)
	if err != nil {
		return nil, err
	}
	fields, _ := body.(map[string]any)

	var reader io.Reader
	contentType := "application/json"
	if len(m.files) == 0 {
		reader = bytes.NewReader(jsonx.Marshal(fields))
	} else {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		contentType = mw.FormDataContentType()
		go func() { pw.CloseWithError(writeMultipart(mw, fields, m.files)) }()
		reader = pr
		defer pr.Close()
	}

	url := fmt.Sprintf("%s/bot%s/%s", strings.TrimRight(c.BaseURL, "/"), token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, apperr.Newf(apperr.Telegram, "Telegram request failed: %s", apperr.Redact(err.Error(), token))
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, apperr.Newf(apperr.Telegram, "Telegram request failed: %s", apperr.Redact(err.Error(), token))
	}
	defer resp.Body.Close()

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, apperr.Newf(apperr.Telegram, "Telegram returned an invalid response for %s", method)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 || !env.OK || len(env.Result) == 0 {
		desc := env.Description
		if desc == "" {
			desc = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		e := apperr.Newf(apperr.Telegram, "Telegram %s failed: %s", method, apperr.Redact(desc, token))
		if env.ErrorCode != 0 {
			e.Details = map[string]any{"telegramErrorCode": env.ErrorCode}
		}
		return nil, e
	}
	return env.Result, nil
}

type attachment struct {
	field    string
	filename string
	reader   io.Reader
}

func writeMultipart(mw *multipart.Writer, fields map[string]any, files []attachment) error {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var value string
		switch v := fields[k].(type) {
		case nil:
			continue
		case string:
			value = v
		case bool, json.Number, int, int64, float64:
			value = string(jsonx.Marshal(v))
		default:
			value = string(jsonx.Marshal(v))
		}
		if err := mw.WriteField(k, value); err != nil {
			return err
		}
	}
	for _, f := range files {
		part, err := mw.CreateFormFile(f.field, f.filename)
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, f.reader); err != nil {
			return err
		}
	}
	return mw.Close()
}

type materializer struct {
	ctx     context.Context
	fetcher *MediaFetcher
	method  string
	files   []attachment
	closers []io.Closer
}

func (m *materializer) close() {
	for _, c := range m.closers {
		c.Close()
	}
}

// walk replaces media sources in v with their wire representation.
func (m *materializer) walk(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		if src, ok := MediaSourceOf(t); ok {
			return m.media(src, t)
		}
		out := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic attachment numbering
		for _, k := range keys {
			r, err := m.walk(t[k])
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			r, err := m.walk(item)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

// MediaSourceOf reports whether obj is a media source and returns its kind.
func MediaSourceOf(obj map[string]any) (string, bool) {
	src, _ := obj["source"].(string)
	switch src {
	case "file_id", "url", "path", "upload":
		return src, true
	}
	if durl, ok := obj["download_url"].(string); ok && durl != "" {
		return "url", true
	}
	return "", false
}

// Upload is the value of a media source's "file" key when source is "upload".
type Upload interface {
	Open() (io.ReadCloser, error)
	Name() string
}

func (m *materializer) media(src string, obj map[string]any) (any, error) {
	value, _ := obj["value"].(string)
	if value == "" {
		if durl, ok := obj["download_url"].(string); ok && durl != "" {
			value = durl
		} else if u, ok := obj["url"].(string); ok && u != "" {
			value = u
		}
	}
	switch src {
	case "file_id":
		if value == "" {
			return nil, apperr.Newf(apperr.Validation, "Media source %s requires a value", src)
		}
		return value, nil
	case "url":
		if value == "" {
			return nil, apperr.Newf(apperr.Validation, "Media source %s requires a value", src)
		}
		maxSize := m.fetcher.MaxMediaSizeBytes
		if m.method == "sendPhoto" || obj["type"] == "photo" {
			maxSize = m.fetcher.MaxPhotoSizeBytes
		}
		data, inferredFilename, _, err := m.fetcher.Fetch(m.ctx, value, maxSize)
		if err != nil {
			return nil, err
		}
		filename := inferredFilename
		if customName, ok := obj["filename"].(string); ok && customName != "" {
			filename = customName
		} else if customName, ok := obj["name"].(string); ok && customName != "" {
			filename = customName
		}
		reader := bytes.NewReader(data)
		name := fmt.Sprintf("attachment_%d", len(m.files))
		m.files = append(m.files, attachment{field: name, filename: filename, reader: reader})
		return "attach://" + name, nil
	}
	var reader io.ReadCloser
	var filename string
	if src == "path" {
		if value == "" {
			return nil, apperr.New(apperr.Validation, "Media source path requires a value")
		}
		f, err := os.Open(value)
		if err != nil {
			return nil, apperr.Newf(apperr.Validation, "Cannot read local file %s", value)
		}
		reader, filename = f, filepath.Base(value)
	} else {
		up, ok := obj["file"].(Upload)
		if !ok {
			return nil, apperr.New(apperr.Validation, "Uploaded media is missing file content")
		}
		r, err := up.Open()
		if err != nil {
			return nil, apperr.New(apperr.Validation, "Cannot read uploaded file")
		}
		reader, filename = r, up.Name()
		if name, ok := obj["filename"].(string); ok && name != "" {
			filename = name
		}
	}
	m.closers = append(m.closers, reader)
	name := fmt.Sprintf("attachment_%d", len(m.files))
	m.files = append(m.files, attachment{field: name, filename: filename, reader: reader})
	return "attach://" + name, nil
}
