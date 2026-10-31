package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/assets"
	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/paths"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/telegram"
	"github.com/dsnbyte/telesend/internal/version"
)

const (
	sessionCookieName = "telesend_session"
	sessionDuration   = 7 * 24 * time.Hour
	maxRequestBytes   = 52 * 1024 * 1024
)

// Server serves the web UI and handles its internal API.
type Server struct {
	app             *app.App
	verifier        *secret.Verifier
	historyPageSize int
	restActive      bool
	mcpActive       bool
}

func loadHistoryPageSize() int {
	if s := os.Getenv("TELESEND_HISTORY_PAGE_SIZE"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	candidates := []string{}
	if p, err := paths.Default(); err == nil && p.ConfigFile != "" {
		candidates = append(candidates, p.ConfigFile)
	}
	candidates = append(candidates, "config.json", filepath.Join("data", "config.json"))

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			UI struct {
				HistoryPageSize int `json:"history_page_size"`
			} `json:"ui"`
			History struct {
				PageSize int `json:"page_size"`
			} `json:"history"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			if cfg.UI.HistoryPageSize > 0 {
				return cfg.UI.HistoryPageSize
			}
			if cfg.History.PageSize > 0 {
				return cfg.History.PageSize
			}
		}
	}
	return 20
}

// NewServer constructs a UI server backed by an App and Verifier.
func NewServer(a *app.App, verifier *secret.Verifier) (*Server, error) {
	if verifier == nil {
		return nil, apperr.New(apperr.Configuration, "TELESEND_UI_PASSWORD or TELESEND_UI_PASSWORD_HASH is required to start the Web UI")
	}
	return &Server{
		app:             a,
		verifier:        verifier,
		historyPageSize: loadHistoryPageSize(),
		restActive:      true,
		mcpActive:       true,
	}, nil
}

// SetServiceStatus updates the reported active status for REST and MCP services.
func (s *Server) SetServiceStatus(restActive, mcpActive bool) {
	s.restActive = restActive
	s.mcpActive = mcpActive
}

// SetHistoryPageSize overrides the pagination size.
func (s *Server) SetHistoryPageSize(n int) {
	if n > 0 {
		s.historyPageSize = n
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > maxRequestBytes {
		writeError(w, apperr.New(apperr.Validation, "Request body is too large"))
		return
	}

	path := r.URL.Path

	// Static assets
	switch path {
	case "/", "/ui", "/ui/":
		s.serveStatic(w, r, "static/index.html", "text/html; charset=utf-8")
		return
	case "/app.css", "/ui/app.css":
		s.serveStatic(w, r, "static/app.css", "text/css; charset=utf-8")
		return
	case "/app.js", "/ui/app.js":
		s.serveStatic(w, r, "static/app.js", "application/javascript; charset=utf-8")
		return
	case "/favicon.ico", "/icon.png", "/ui/icon.png", "/ui/favicon.ico", "/apple-touch-icon.png", "/telesend-icon.png":
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Length", strconv.Itoa(len(assets.Icon)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(assets.Icon)
		}
		return
	}

	// API routes
	if strings.HasPrefix(path, "/api/ui/") {
		w.Header().Set("Cache-Control", "no-store")
		apiPath := strings.TrimPrefix(path, "/api/ui")
		if err := s.routeAPI(w, r, apiPath); err != nil {
			writeError(w, err)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, path, contentType string) {
	data, err := staticFS.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) routeAPI(w http.ResponseWriter, r *http.Request, path string) error {
	// Public auth routes
	switch path {
	case "/auth/status":
		return s.handleAuthStatus(w, r)
	case "/auth/login":
		if r.Method != http.MethodPost {
			return apperr.New(apperr.Validation, "Method not allowed")
		}
		return s.handleAuthLogin(w, r)
	}

	// Protected routes require valid session
	sessionToken := s.getSessionToken(r)
	valid, err := s.app.UIAuth.ValidateSession(sessionToken)
	if err != nil {
		return err
	}
	if !valid {
		return apperr.New(apperr.Unauthorized, "Authentication required")
	}

	// Protected auth routes
	switch path {
	case "/auth/logout":
		if r.Method != http.MethodPost {
			return apperr.New(apperr.Validation, "Method not allowed")
		}
		return s.handleAuthLogout(w, r, sessionToken)
	}

	// Dashboard Overview
	if path == "/overview" && r.Method == http.MethodGet {
		return s.handleOverview(w, r)
	}

	// Server and MCP info
	if path == "/server-info" && r.Method == http.MethodGet {
		return s.handleServerInfo(w, r)
	}

	// Hash tool
	if path == "/hash" && r.Method == http.MethodPost {
		return s.handleHash(w, r)
	}

	// Database export / import
	if path == "/db/export" && r.Method == http.MethodGet {
		return s.handleDBExport(w, r)
	}
	if path == "/db/import" && r.Method == http.MethodPost {
		return s.handleDBImport(w, r)
	}

	// Bots
	if path == "/bots" {
		switch r.Method {
		case http.MethodGet:
			return s.handleListBots(w, r)
		case http.MethodPost:
			return s.handleAddBot(w, r)
		}
	}

	if strings.HasPrefix(path, "/bots/") {
		sub := strings.TrimPrefix(path, "/bots/")
		username, action, hasAction := strings.Cut(sub, "/")
		username, _ = url.PathUnescape(username)
		if hasAction && action == "default" && r.Method == http.MethodPost {
			return s.handleSetDefaultBot(w, r, username)
		}
		if !hasAction {
			switch r.Method {
			case http.MethodPatch:
				return s.handleUpdateBot(w, r, username)
			case http.MethodDelete:
				return s.handleDeleteBot(w, r, username)
			}
		}
	}

	// Aliases
	if path == "/aliases" {
		switch r.Method {
		case http.MethodGet:
			return s.handleListAliases(w, r)
		case http.MethodPost:
			return s.handleAddAlias(w, r)
		}
	}

	if strings.HasPrefix(path, "/aliases/") {
		name, _ := url.PathUnescape(strings.TrimPrefix(path, "/aliases/"))
		if !strings.Contains(name, "/") {
			switch r.Method {
			case http.MethodPatch:
				return s.handleUpdateAlias(w, r, name)
			case http.MethodDelete:
				return s.handleDeleteAlias(w, r, name)
			}
		}
	}

	// Send message
	if path == "/send" && r.Method == http.MethodPost {
		return s.handleSend(w, r)
	}

	// History
	if path == "/history" {
		switch r.Method {
		case http.MethodGet:
			return s.handleListHistory(w, r)
		case http.MethodDelete:
			return s.handleClearHistory(w, r)
		}
	}

	if strings.HasPrefix(path, "/history/") {
		idStr := strings.TrimPrefix(path, "/history/")
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && id > 0 {
			switch r.Method {
			case http.MethodGet:
				return s.handleGetHistory(w, r, id)
			case http.MethodDelete:
				return s.handleDeleteHistory(w, r, id)
			}
		}
	}

	return apperr.New(apperr.NotFound, "Endpoint not found")
}

func (s *Server) getSessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return r.Header.Get("X-UI-Session")
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Auth Handlers

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) error {
	token := s.getSessionToken(r)
	authenticated, _ := s.app.UIAuth.ValidateSession(token)

	writeJSON(w, http.StatusOK, map[string]any{
		"initialized":   true,
		"authenticated": authenticated,
		"version":       version.Version,
	})
	return nil
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return apperr.New(apperr.Validation, "Invalid request body")
	}
	if !s.verifier.Verify(body.Password) {
		return apperr.New(apperr.Unauthorized, "Incorrect password")
	}

	token, err := generateToken()
	if err != nil {
		return err
	}
	if err := s.app.UIAuth.CreateSession(token, sessionDuration); err != nil {
		return err
	}
	s.setSessionCookie(w, r, token, int(sessionDuration.Seconds()))

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request, token string) error {
	_ = s.app.UIAuth.DeleteSession(token)
	s.setSessionCookie(w, r, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

// Overview & Info Handlers

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) error {
	bots, err := s.app.Bots.List()
	if err != nil {
		return err
	}
	if bots == nil {
		bots = []store.Bot{}
	}
	aliases, err := s.app.Aliases.List()
	if err != nil {
		return err
	}
	if aliases == nil {
		aliases = []store.Alias{}
	}

	histCount, _ := s.app.History.Count()

	writeJSON(w, http.StatusOK, map[string]any{
		"version":              version.Version,
		"bots":                 bots,
		"aliases":              aliases,
		"historyCount":         histCount,
		"historyRetentionDays": s.app.History.RetentionDays(),
	})
	return nil
}

func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) error {
	type catItem struct {
		Type        string   `json:"type"`
		Method      string   `json:"method"`
		Description string   `json:"description"`
		Required    []string `json:"required"`
		AllowURL    bool     `json:"allowUrl"`
	}
	catalog := make([]catItem, len(telegram.Catalog))
	for i, op := range telegram.Catalog {
		catalog[i] = catItem{
			Type:        op.Type,
			Method:      op.Method,
			Description: op.Description,
			Required:    op.Required,
			AllowURL:    op.AllowURL,
		}
	}

	mcpTools := []map[string]string{
		{"name": "list_bots", "description": "List all registered Telegram bots and identify the default."},
		{"name": "list_aliases", "description": "List recipient aliases and chat IDs, optionally filtered by type."},
		{"name": "send_text", "description": "Send a plain, HTML, or Markdown text message to an alias or chat ID."},
		{"name": "send_photo", "description": "Send a photo with optional caption from a URL, Telegram file ID, or uploaded file."},
		{"name": "send_document", "description": "Send a document or file with optional caption from a URL, Telegram file ID, or uploaded file."},
		{"name": "send_media_group", "description": "Send an album of photos or documents."},
		{"name": "send_message", "description": "Send any supported Telegram message type using full parameter payload."},
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"botApiVersion":  telegram.BotAPIVersion,
		"databaseFile":   filepath.Base(s.app.DatabasePath),
		"restActive":     s.restActive,
		"mcpActive":      s.mcpActive,
		"catalog":        catalog,
		"mcpTools":       mcpTools,
	})
	return nil
}

// History Handlers

func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	limit := s.historyPageSize
	if limit <= 0 {
		limit = 20
	}
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	var offset int
	if o := q.Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	searchQuery := q.Get("q")
	if searchQuery == "" {
		searchQuery = q.Get("search")
	}
	filter := store.HistoryFilter{
		Limit:       limit,
		Offset:      offset,
		MessageType: q.Get("type"),
		Bot:         q.Get("bot"),
		Recipient:   q.Get("recipient"),
		Query:       searchQuery,
	}
	entries, err := s.app.History.List(filter)
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []store.HistoryEntry{}
	}
	total, _ := s.app.History.Count()
	filteredTotal := total
	if filter.MessageType != "" || filter.Bot != "" || filter.Recipient != "" || filter.Query != "" {
		filteredTotal, _ = s.app.History.CountFiltered(filter)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":          entries,
		"total":         total,
		"filteredTotal": filteredTotal,
		"pageSize":      limit,
		"offset":        offset,
		"hasMore":       (offset + len(entries)) < filteredTotal,
		"retentionDays": s.app.History.RetentionDays(),
	})
	return nil
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request, id int64) error {
	entry, err := s.app.History.Get(id)
	if err != nil {
		return err
	}
	if entry == nil {
		return apperr.Newf(apperr.NotFound, "History entry %d was not found", id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entry})
	return nil
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request, id int64) error {
	q := r.URL.Query()
	deleteTelegram := q.Get("delete_telegram") == "true" || q.Get("delete_telegram") == "1" ||
		q.Get("delete-telegram") == "true" || q.Get("delete-telegram") == "1"
	res, err := s.app.DeleteHistory(r.Context(), id, deleteTelegram)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
	return nil
}

func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) error {
	deleted, err := s.app.History.Clear()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": deleted}})
	return nil
}

// Bot Management Handlers

func (s *Server) handleListBots(w http.ResponseWriter, r *http.Request) error {
	bots, err := s.app.Bots.List()
	if err != nil {
		return err
	}
	if bots == nil {
		bots = []store.Bot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bots})
	return nil
}

func (s *Server) handleAddBot(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		return apperr.New(apperr.Validation, "Field \"token\" is required")
	}
	bot, err := s.app.RegisterBot(r.Context(), body.Token)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": bot})
	return nil
}

func (s *Server) handleUpdateBot(w http.ResponseWriter, r *http.Request, username string) error {
	var body struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return apperr.New(apperr.Validation, "Invalid request body")
	}
	bot, err := s.app.UpdateBot(r.Context(), username, store.BotInput{
		Name:  body.Name,
		Token: body.Token,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bot})
	return nil
}

func (s *Server) handleSetDefaultBot(w http.ResponseWriter, r *http.Request, username string) error {
	bot, err := s.app.Bots.SetDefault(username)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bot})
	return nil
}

func (s *Server) handleDeleteBot(w http.ResponseWriter, r *http.Request, username string) error {
	bot, err := s.app.Bots.Remove(username)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bot})
	return nil
}

// Alias Management Handlers

func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request) error {
	filter := r.URL.Query().Get("filter")
	aliases, err := s.app.Aliases.List(filter)
	if err != nil {
		return err
	}
	if aliases == nil {
		aliases = []store.Alias{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": aliases})
	return nil
}

func (s *Server) handleAddAlias(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name            string `json:"name"`
		ChatID          string `json:"chatId"`
		MessageThreadID *int   `json:"messageThreadId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return apperr.New(apperr.Validation, "Invalid request body")
	}
	alias, err := s.app.Aliases.Create(store.AliasInput{
		Name:            body.Name,
		ChatID:          body.ChatID,
		MessageThreadID: body.MessageThreadID,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": alias})
	return nil
}

func (s *Server) handleUpdateAlias(w http.ResponseWriter, r *http.Request, name string) error {
	existing, err := s.app.Aliases.Find(name)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperr.Newf(apperr.NotFound, "Alias %q was not found", name)
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return apperr.New(apperr.Validation, "Invalid request body")
	}

	in := store.AliasInput{
		Name:            existing.Name,
		ChatID:          existing.ChatID,
		MessageThreadID: existing.MessageThreadID,
	}
	if n, ok := body["name"].(string); ok && strings.TrimSpace(n) != "" {
		in.Name = n
	}
	if c, ok := body["chatId"].(string); ok && strings.TrimSpace(c) != "" {
		in.ChatID = c
	}
	if t, ok := body["messageThreadId"]; ok {
		if t == nil {
			in.MessageThreadID = nil
		} else if num, ok := t.(float64); ok && num > 0 {
			v := int(num)
			in.MessageThreadID = &v
		} else {
			return apperr.New(apperr.Validation, "messageThreadId must be a positive integer or null")
		}
	}

	updated, err := s.app.Aliases.Update(name, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
	return nil
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request, name string) error {
	alias, err := s.app.Aliases.Remove(name)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": alias})
	return nil
}

// Send Message Handler

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)

	var (
		msgType  string
		to       string
		bot      string
		threadID int
		payload  map[string]any
	)

	if mediaType == "multipart/form-data" {
		reader, err := r.MultipartReader()
		if err != nil {
			return apperr.New(apperr.Validation, "Failed to read multipart payload")
		}
		payload = map[string]any{}
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			name := part.FormName()
			if name == "" {
				part.Close()
				continue
			}

			if part.FileName() != "" {
				data, err := io.ReadAll(part)
				part.Close()
				if err != nil {
					return err
				}
				payload[name] = map[string]any{
					"source":   "upload",
					"file":     bytesUpload{filename: part.FileName(), data: data},
					"filename": part.FileName(),
				}
				continue
			}

			bytes, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				return err
			}
			val := string(bytes)
			switch name {
			case "type":
				msgType = val
			case "to":
				to = val
			case "bot":
				bot = val
			case "messageThreadId", "threadId":
				if val != "" {
					threadID, _ = strconv.Atoi(val)
				}
			case "silent":
				if val == "true" || val == "1" {
					payload["disable_notification"] = true
				}
			case "parseMode", "parse_mode":
				if val != "" {
					payload["parse_mode"] = val
				}
			case "caption":
				if val != "" {
					payload["caption"] = val
				}
			case "data":
				// Optional raw JSON fields to merge
				if val != "" {
					var extra map[string]any
					if json.Unmarshal([]byte(val), &extra) == nil && extra != nil {
						for k, v := range extra {
							payload[k] = v
						}
					}
				}
			default:
				payload[name] = val
			}
		}
	} else {
		var req struct {
			Type     string         `json:"type"`
			To       string         `json:"to"`
			Bot      string         `json:"bot"`
			ThreadID int            `json:"threadId"`
			Silent   bool           `json:"silent"`
			Payload  map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return apperr.New(apperr.Validation, "Invalid request JSON")
		}
		msgType = req.Type
		to = req.To
		bot = req.Bot
		threadID = req.ThreadID
		payload = req.Payload
		if payload == nil {
			payload = map[string]any{}
		}
		if req.Silent {
			payload["disable_notification"] = true
		}
	}

	if msgType == "" {
		msgType = "text"
	}
	if to == "" {
		return apperr.New(apperr.Validation, "Field \"to\" is required")
	}

	// Reject any client-provided filesystem paths
	if err := rejectServerPaths(payload); err != nil {
		return err
	}

	delivery, err := s.app.Send(r.Context(), app.SendRequest{
		Type:     msgType,
		To:       to,
		Bot:      bot,
		ThreadID: threadID,
		Payload:  payload,
	})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": delivery})
	return nil
}

func rejectServerPaths(v any) error {
	switch val := v.(type) {
	case map[string]any:
		if src, ok := val["source"].(string); ok && src == "path" {
			return apperr.New(apperr.Validation, "UI requests cannot use server filesystem paths")
		}
		for _, child := range val {
			if err := rejectServerPaths(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range val {
			if err := rejectServerPaths(child); err != nil {
				return err
			}
		}
	}
	return nil
}

type bytesUpload struct {
	filename string
	data     []byte
}

func (b bytesUpload) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.data)), nil
}

func (b bytesUpload) Name() string {
	return b.filename
}

// Hash Utility Handler

func (s *Server) handleHash(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Type  string `json:"type"` // "key" or "password"
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return apperr.New(apperr.Validation, "Invalid request body")
	}
	val := strings.TrimSpace(body.Value)
	if val == "" {
		return apperr.New(apperr.Validation, "Value to hash cannot be empty")
	}

	rawHash, err := secret.Hash(val)
	if err != nil {
		return err
	}
	encoded := secret.Encode(rawHash)

	envKey := "TELESEND_API_KEY_HASH"
	switch body.Type {
	case "password", "mcp":
		envKey = "TELESEND_MCP_PASSWORD_HASH"
	case "ui-password", "ui_password", "ui":
		envKey = "TELESEND_UI_PASSWORD_HASH"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hash":     encoded,
		"envKey":   envKey,
		"envLine":  fmt.Sprintf("%s=%s", envKey, encoded),
		"envLines": map[string]string{
			"apiKey": fmt.Sprintf("TELESEND_API_KEY_HASH=%s", encoded),
			"mcp":    fmt.Sprintf("TELESEND_MCP_PASSWORD_HASH=%s", encoded),
			"ui":     fmt.Sprintf("TELESEND_UI_PASSWORD_HASH=%s", encoded),
		},
	})
	return nil
}

// Database Export & Import Handlers

func (s *Server) handleDBExport(w http.ResponseWriter, r *http.Request) error {
	sqlDump, err := s.app.ExportDatabase()
	if err != nil {
		return err
	}
	if r.URL.Query().Get("download") == "true" || r.URL.Query().Get("download") == "1" {
		filename := fmt.Sprintf("telesend-backup-%s.sql", time.Now().UTC().Format("2006-01-02-150405"))
		w.Header().Set("Content-Type", "application/sql; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		_, _ = w.Write([]byte(sqlDump))
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sql":  sqlDump,
		"size": len(sqlDump),
	})
	return nil
}

func (s *Server) handleDBImport(w http.ResponseWriter, r *http.Request) error {
	var script string
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			script = r.FormValue("sql")
			if strings.TrimSpace(script) == "" {
				return apperr.New(apperr.Validation, "No SQL file or script provided")
			}
		} else {
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				return apperr.New(apperr.Validation, "Failed to read uploaded SQL file")
			}
			script = string(data)
		}
	} else if strings.HasPrefix(ct, "application/json") {
		var body struct {
			SQL string `json:"sql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return apperr.New(apperr.Validation, "Invalid request body")
		}
		script = body.SQL
	} else {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return apperr.New(apperr.Validation, "Failed to read request body")
		}
		script = string(data)
	}

	script = strings.TrimSpace(script)
	if script == "" {
		return apperr.New(apperr.Validation, "SQL script cannot be empty")
	}

	currentSession := s.getSessionToken(r)

	if err := s.app.ImportDatabase(script); err != nil {
		return apperr.New(apperr.Validation, fmt.Sprintf("Import error: %v", err))
	}

	if currentSession != "" {
		valid, _ := s.app.UIAuth.ValidateSession(currentSession)
		if !valid {
			_ = s.app.UIAuth.CreateSession(currentSession, sessionDuration)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Database imported successfully. All data was replaced.",
	})
	return nil
}

// Helpers

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		status = appErr.HTTPStatus()
	}
	msg := apperr.Redact(err.Error())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}
