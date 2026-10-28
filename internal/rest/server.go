// Package rest implements the authenticated Telesend HTTP REST API.
package rest

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/jsonx"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/telegram"
)

const maxRequestBytes = 52 * 1024 * 1024

// Server is an http.Handler implementing the REST API.
type Server struct {
	app      *app.App
	verifier *secret.Verifier
}

// NewServer builds a REST handler. verifier must not be nil.
func NewServer(a *app.App, verifier *secret.Verifier) (*Server, error) {
	if verifier == nil {
		return nil, apperr.New(apperr.Configuration, "TELESEND_API_KEY or TELESEND_API_KEY_HASH is required to start the REST server")
	}
	return &Server{app: a, verifier: verifier}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Authenticate x-api-key
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" || !s.verifier.Verify(apiKey) {
		writeError(w, apperr.New(apperr.Unauthorized, "Invalid API key"), s.verifier.Secrets()...)
		return
	}

	// 2. Request body size check
	if r.ContentLength > maxRequestBytes {
		writeError(w, apperr.New(apperr.Validation, "Request body is too large"), s.verifier.Secrets()...)
		return
	}

	if err := s.route(w, r); err != nil {
		writeError(w, err, s.verifier.Secrets()...)
	}
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.Path
	method := r.Method

	if method == http.MethodGet && path == "/health" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return nil
	}

	// Bots
	if path == "/bots" {
		switch method {
		case http.MethodGet:
			bots, err := s.app.Bots.List()
			if err != nil {
				return err
			}
			if bots == nil {
				bots = []store.Bot{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": bots})
			return nil
		case http.MethodPost:
			var body struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
				return apperr.New(apperr.Validation, "Field \"token\" is required")
			}
			bot, err := s.app.RegisterBot(r.Context(), body.Token)
			if err != nil {
				return err
			}
			writeJSON(w, http.StatusCreated, map[string]any{"data": bot})
			return nil
		}
	}

	if strings.HasPrefix(path, "/bots/") {
		rest := path[len("/bots/"):]
		username, defaultSuffix, hasDefault := strings.Cut(rest, "/")
		username, _ = url.PathUnescape(username)
		if hasDefault && defaultSuffix == "default" && method == http.MethodPost {
			bot, err := s.app.Bots.SetDefault(username)
			if err != nil {
				return err
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": bot})
			return nil
		}
		if !hasDefault && method == http.MethodDelete {
			bot, err := s.app.Bots.Remove(username)
			if err != nil {
				return err
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": bot})
			return nil
		}
	}

	// Aliases
	if path == "/aliases" {
		switch method {
		case http.MethodGet:
			aliases, err := s.app.Aliases.List()
			if err != nil {
				return err
			}
			if aliases == nil {
				aliases = []store.Alias{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": aliases})
			return nil
		case http.MethodPost:
			var body struct {
				Name            string `json:"name"`
				ChatID          string `json:"chatId"`
				MessageThreadID *int   `json:"messageThreadId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return apperr.New(apperr.Validation, "Request body must be a JSON object")
			}
			if body.Name == "" {
				return apperr.New(apperr.Validation, "Field \"name\" is required")
			}
			if body.ChatID == "" {
				return apperr.New(apperr.Validation, "Field \"chatId\" is required")
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
	}

	if strings.HasPrefix(path, "/aliases/") {
		name, _ := url.PathUnescape(path[len("/aliases/"):])
		if !strings.Contains(name, "/") {
			switch method {
			case http.MethodDelete:
				alias, err := s.app.Aliases.Remove(name)
				if err != nil {
					return err
				}
				writeJSON(w, http.StatusOK, map[string]any{"data": alias})
				return nil
			case http.MethodPatch:
				existing, err := s.app.Aliases.Find(name)
				if err != nil {
					return err
				}
				if existing == nil {
					return apperr.Newf(apperr.NotFound, "Alias %q was not found", name)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					return apperr.New(apperr.Validation, "Request body must be a JSON object")
				}
				in := store.AliasInput{
					Name:            existing.Name,
					ChatID:          existing.ChatID,
					MessageThreadID: existing.MessageThreadID,
				}
				if n, ok := body["name"].(string); ok && n != "" {
					in.Name = n
				}
				if c, ok := body["chatId"].(string); ok && c != "" {
					in.ChatID = c
				}
				if t, ok := body["messageThreadId"]; ok {
					if t == nil {
						in.MessageThreadID = nil
					} else if n, ok := t.(float64); ok && n > 0 {
						v := int(n)
						in.MessageThreadID = &v
					} else {
						return apperr.New(apperr.Validation, "Field \"messageThreadId\" must be a positive integer")
					}
				}
				updated, err := s.app.Aliases.Update(name, in)
				if err != nil {
					return err
				}
				writeJSON(w, http.StatusOK, map[string]any{"data": updated})
				return nil
			}
		}
	}

	// Messages
	if strings.HasPrefix(path, "/messages/") && method == http.MethodPost {
		msgType, _ := url.PathUnescape(path[len("/messages/"):])
		if strings.Contains(msgType, "/") {
			return apperr.New(apperr.NotFound, "Endpoint not found")
		}
		if _, ok := telegram.Get(msgType); !ok {
			return apperr.Newf(apperr.NotFound, "Message type %q was not found", msgType)
		}
		req, err := s.readMessageRequest(r)
		if err != nil {
			return err
		}
		if err := rejectServerPaths(req.Payload); err != nil {
			return err
		}
		res, err := s.app.Send(r.Context(), app.SendRequest{
			Type:     msgType,
			To:       req.To,
			Bot:      req.Bot,
			ThreadID: req.ThreadID,
			Payload:  req.Payload,
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": res})
		return nil
	}

	// History
	if path == "/history" || path == "/messages/history" {
		switch method {
		case http.MethodGet:
			q := r.URL.Query()
			var limit int
			if l := q.Get("limit"); l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 {
					limit = n
				}
			}
			entries, err := s.app.History.List(store.HistoryFilter{
				Limit:       limit,
				MessageType: q.Get("type"),
				Bot:         q.Get("bot"),
				Recipient:   q.Get("recipient"),
			})
			if err != nil {
				return err
			}
			if entries == nil {
				entries = []store.HistoryEntry{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": entries})
			return nil
		case http.MethodDelete:
			deleted, err := s.app.History.Clear()
			if err != nil {
				return err
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": deleted}})
			return nil
		}
	}

	if strings.HasPrefix(path, "/history/") {
		idStr := path[len("/history/"):]
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && id > 0 {
			switch method {
			case http.MethodGet:
				entry, err := s.app.History.Get(id)
				if err != nil {
					return err
				}
				if entry == nil {
					return apperr.Newf(apperr.NotFound, "History entry %d was not found", id)
				}
				writeJSON(w, http.StatusOK, map[string]any{"data": entry})
				return nil
			case http.MethodDelete:
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
		}
	}

	return apperr.New(apperr.NotFound, "Endpoint not found")
}

type messageInput struct {
	To       string
	Bot      string
	ThreadID int
	Payload  map[string]any
}

func (s *Server) readMessageRequest(r *http.Request) (messageInput, error) {
	ct, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(ct, "multipart/") {
		var body struct {
			To              string         `json:"to"`
			Bot             string         `json:"bot"`
			MessageThreadID int            `json:"messageThreadId"`
			Payload         map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return messageInput{}, apperr.New(apperr.Validation, "Request body must be valid JSON")
		}
		if body.To == "" {
			return messageInput{}, apperr.New(apperr.Validation, "Field \"to\" is required")
		}
		if body.Payload == nil {
			return messageInput{}, apperr.New(apperr.Validation, "Field \"payload\" must be an object")
		}
		return messageInput{
			To:       body.To,
			Bot:      body.Bot,
			ThreadID: body.MessageThreadID,
			Payload:  body.Payload,
		}, nil
	}

	// Multipart upload
	mr := multipart.NewReader(r.Body, params["boundary"])
	form, err := mr.ReadForm(maxRequestBytes)
	if err != nil {
		return messageInput{}, apperr.New(apperr.Validation, "Invalid multipart payload")
	}
	defer form.RemoveAll()

	to := formValue(form, "to")
	if to == "" {
		return messageInput{}, apperr.New(apperr.Validation, "Field \"to\" is required")
	}
	var thread int
	if t := formValue(form, "messageThreadId"); t != "" {
		var err error
		thread, err = parsePositiveInt(t, "messageThreadId")
		if err != nil {
			return messageInput{}, err
		}
	}
	payload := map[string]any{}
	if rawPayload := formValue(form, "payload"); rawPayload != "" {
		if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
			return messageInput{}, apperr.New(apperr.Validation, "Multipart field \"payload\" must be a JSON object")
		}
	}

	// Resolve upload references
	payload = resolveUploadReferences(payload, form).(map[string]any)

	// Add any uploaded files not already in payload
	for field, files := range form.File {
		if field == "to" || field == "bot" || field == "messageThreadId" || field == "payload" {
			continue
		}
		if len(files) > 0 && payload[field] == nil {
			payload[field] = map[string]any{
				"source":   "upload",
				"file":     multipartUpload{header: files[0]},
				"filename": files[0].Filename,
			}
		}
	}

	return messageInput{
		To:       to,
		Bot:      formValue(form, "bot"),
		ThreadID: thread,
		Payload:  payload,
	}, nil
}

type multipartUpload struct {
	header *multipart.FileHeader
}

func (m multipartUpload) Open() (io.ReadCloser, error) { return m.header.Open() }
func (m multipartUpload) Name() string                 { return m.header.Filename }

func resolveUploadReferences(v any, form *multipart.Form) any {
	switch t := v.(type) {
	case map[string]any:
		src, _ := t["source"].(string)
		val, _ := t["value"].(string)
		if src == "upload" && val != "" {
			files := form.File[val]
			if len(files) == 0 {
				panic(apperr.Newf(apperr.Validation, "Multipart file %q is required", val))
			}
			return map[string]any{
				"source":   "upload",
				"file":     multipartUpload{header: files[0]},
				"filename": files[0].Filename,
			}
		}
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = resolveUploadReferences(item, form)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = resolveUploadReferences(item, form)
		}
		return out
	}
	return v
}

func rejectServerPaths(v any) error {
	switch t := v.(type) {
	case map[string]any:
		if t["source"] == "path" {
			return apperr.New(apperr.Validation, "REST requests cannot use server filesystem paths")
		}
		for _, item := range t {
			if err := rejectServerPaths(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := rejectServerPaths(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func formValue(form *multipart.Form, name string) string {
	if vals := form.Value[name]; len(vals) > 0 {
		return strings.TrimSpace(vals[0])
	}
	return ""
}

func parsePositiveInt(val, name string) (int, error) {
	var n int
	for _, c := range val {
		if c < '0' || c > '9' {
			return 0, apperr.Newf(apperr.Validation, "Field %q must be a positive integer", name)
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, apperr.Newf(apperr.Validation, "Field %q must be a positive integer", name)
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(jsonx.Marshal(v))
}

func writeError(w http.ResponseWriter, err error, secrets ...string) {
	e := apperr.From(err, secrets...)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := e.HTTPStatus()
	w.WriteHeader(status)
	body := map[string]any{"error": map[string]any{"code": e.Code, "message": e.Message}}
	if e.Details != nil {
		body["error"].(map[string]any)["details"] = e.Details
	}
	w.Write(jsonx.Marshal(body))
}
