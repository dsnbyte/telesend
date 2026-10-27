package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/assets"
	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/jsonx"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/version"
)

const (
	maxMCPBodyBytes      = 1024 * 1024
	accessTokenTTL       = 60 * 60         // 1 hour
	authorizationCodeTTL = 5 * 60          // 5 minutes
	refreshTokenTTL      = 30 * 24 * 60 * 60 // 30 days
)

var supportedScopes = []string{"mcp:read", "mcp:send"}

// RemoteConfig configures the HTTP MCP listener.
type RemoteConfig struct {
	PublicURL     *url.URL
	OwnerVerifier *secret.Verifier
	Now           func() int64
}

// RemoteServer is an HTTP handler for remote MCP and OAuth.
type RemoteServer struct {
	app    *app.App
	cfg    RemoteConfig
	now    func() int64
	tokens func() string
}

// NewRemoteServer constructs a RemoteServer.
func NewRemoteServer(a *app.App, cfg RemoteConfig) (*RemoteServer, error) {
	if cfg.PublicURL == nil || cfg.PublicURL.Scheme != "https" || cfg.PublicURL.Host == "" ||
		cfg.PublicURL.Path != "" && cfg.PublicURL.Path != "/" ||
		cfg.PublicURL.User != nil || cfg.PublicURL.RawQuery != "" || cfg.PublicURL.Fragment != "" {
		return nil, apperr.New(apperr.Configuration, "TELESEND_MCP_PUBLIC_URL must be an HTTPS origin")
	}
	if cfg.OwnerVerifier == nil {
		return nil, apperr.New(apperr.Configuration, "TELESEND_MCP_OWNER_PASSWORD or TELESEND_MCP_OWNER_PASSWORD_HASH is required")
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = func() int64 { return time.Now().Unix() }
	}
	return &RemoteServer{
		app:    a,
		cfg:    cfg,
		now:    nowFn,
		tokens: func() string { return randToken() },
	}, nil
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *RemoteServer) endpoint(path string) string {
	u := *s.cfg.PublicURL
	u.Path = path
	return u.String()
}

// ServeHTTP routes OAuth endpoints, discovery, and /mcp.
func (s *RemoteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 1. Static icons
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		switch path {
		case "/icon.png", "/favicon.ico", "/apple-touch-icon.png", "/telesend-icon.png":
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
			w.Header().Set("Content-Length", strconv.Itoa(len(assets.Icon)))
			if r.Method == http.MethodGet {
				w.Write(assets.Icon)
			}
			return
		}
	}

	// 2. Discovery
	if r.Method == http.MethodGet {
		switch path {
		case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
			writeJSON(w, http.StatusOK, map[string]any{
				"resource":              s.endpoint("/mcp"),
				"authorization_servers": []string{urlOrigin(s.cfg.PublicURL)},
				"scopes_supported":      supportedScopes,
				"logo_uri":              s.endpoint("/icon.png"),
				"icon_uri":              s.endpoint("/icon.png"),
			})
			return
		case "/.well-known/oauth-authorization-server", "/.well-known/openid-configuration":
			writeJSON(w, http.StatusOK, map[string]any{
				"issuer":                                urlOrigin(s.cfg.PublicURL),
				"authorization_endpoint":                s.endpoint("/authorize"),
				"token_endpoint":                        s.endpoint("/token"),
				"registration_endpoint":                 s.endpoint("/register"),
				"revocation_endpoint":                   s.endpoint("/revoke"),
				"response_types_supported":              []string{"code"},
				"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
				"token_endpoint_auth_methods_supported": []string{"none"},
				"code_challenge_methods_supported":      []string{"S256"},
				"scopes_supported":                      supportedScopes,
				"logo_uri":                              s.endpoint("/icon.png"),
				"icon_uri":                              s.endpoint("/icon.png"),
			})
			return
		}
	}

	// 3. OAuth registration
	if path == "/register" && r.Method == http.MethodPost {
		s.handleRegister(w, r)
		return
	}

	// 4. OAuth authorize
	if path == "/authorize" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.handleAuthorize(w, r)
		return
	}

	// 5. OAuth token
	if path == "/token" && r.Method == http.MethodPost {
		s.handleToken(w, r)
		return
	}

	// 6. OAuth revoke
	if path == "/revoke" && r.Method == http.MethodPost {
		s.handleRevoke(w, r)
		return
	}

	// 7. MCP JSON-RPC endpoint
	if path == "/mcp" {
		s.handleMCP(w, r)
		return
	}

	// Unknown path
	safeError(w, "not_found", "Route not found", http.StatusNotFound)
}

func (s *RemoteServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.RedirectURIs) == 0 || len(body.RedirectURIs) > 10 {
		oauthError(w, "invalid_redirect_uri", "redirect_uris must contain valid callback URLs", http.StatusBadRequest)
		return
	}
	for _, u := range body.RedirectURIs {
		if !validRedirectURI(u) {
			oauthError(w, "invalid_redirect_uri", "redirect_uris must contain valid callback URLs", http.StatusBadRequest)
			return
		}
	}
	if body.AuthMethod != "" && body.AuthMethod != "none" {
		oauthError(w, "invalid_client_metadata", "Only public PKCE clients are supported", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	client := store.OAuthClient{
		ID:           "client_" + s.tokens(),
		Name:         name,
		RedirectURIs: slices.Compact(body.RedirectURIs),
	}
	if err := s.app.OAuth.RegisterClient(client, s.now()); err != nil {
		safeError(w, "internal", "Registration failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ID,
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": "none",
	})
}

func (s *RemoteServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	var query url.Values
	var password string
	if r.Method == http.MethodGet {
		query = r.URL.Query()
	} else {
		if err := r.ParseForm(); err != nil {
			oauthError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
			return
		}
		query = r.PostForm
		password = r.PostFormValue("password")
	}

	clientID := query.Get("client_id")
	client, err := s.app.OAuth.FindClient(clientID)
	if err != nil || client == nil {
		oauthError(w, "invalid_request", "Client or redirect URI is invalid", http.StatusBadRequest)
		return
	}
	redirectURI := query.Get("redirect_uri")
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		oauthError(w, "invalid_request", "Client or redirect URI is invalid", http.StatusBadRequest)
		return
	}
	if query.Get("response_type") != "code" {
		oauthError(w, "unsupported_response_type", "response_type must be code", http.StatusBadRequest)
		return
	}
	challenge := query.Get("code_challenge")
	if query.Get("code_challenge_method") != "S256" || !isBase64URL(challenge, 43, 128) {
		oauthError(w, "invalid_request", "PKCE S256 is required", http.StatusBadRequest)
		return
	}
	if res := query.Get("resource"); res != "" && res != s.endpoint("/mcp") {
		oauthError(w, "invalid_target", "OAuth resource is invalid", http.StatusBadRequest)
		return
	}
	scopes := parseScopes(query.Get("scope"))
	if scopes == nil {
		oauthError(w, "invalid_scope", "Requested OAuth scope is invalid", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		page := strings.Replace(consentHTML, "{{CLIENT_NAME}}", html.EscapeString(client.Name), 1)
		page = strings.Replace(page, "{{SCOPES}}", html.EscapeString(strings.Join(scopes, ", ")), 1)
		page = strings.Replace(page, "{{CLIENT_ID}}", html.EscapeString(clientID), 1)
		page = strings.Replace(page, "{{REDIRECT_URI}}", html.EscapeString(redirectURI), 1)
		page = strings.Replace(page, "{{SCOPE_VAL}}", html.EscapeString(strings.Join(scopes, " ")), 1)
		page = strings.Replace(page, "{{STATE}}", html.EscapeString(query.Get("state")), 1)
		page = strings.Replace(page, "{{CHALLENGE}}", html.EscapeString(challenge), 1)
		io.WriteString(w, page)
		return
	}

	// POST: verify password
	if !s.cfg.OwnerVerifier.Verify(password) {
		oauthError(w, "access_denied", "Owner authentication failed", http.StatusUnauthorized)
		return
	}
	grantID := "grant_" + s.tokens()
	now := s.now()
	if err := s.app.OAuth.CreateGrant(grantID, clientID, scopes, now); err != nil {
		safeError(w, "internal", "Failed to create grant", http.StatusInternalServerError)
		return
	}
	code := "code_" + s.tokens()
	if err := s.app.OAuth.StoreAuthorizationCode(digest(code), grantID, redirectURI, challenge, now+authorizationCodeTTL); err != nil {
		safeError(w, "internal", "Failed to store code", http.StatusInternalServerError)
		return
	}
	redir, err := url.Parse(redirectURI)
	if err != nil {
		safeError(w, "internal", "Invalid redirect URI", http.StatusInternalServerError)
		return
	}
	q := redir.Query()
	q.Set("code", code)
	if state := query.Get("state"); state != "" {
		q.Set("state", state)
	}
	redir.RawQuery = q.Encode()
	http.Redirect(w, r, redir.String(), http.StatusFound)
}

func (s *RemoteServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if !isForm(r) {
		oauthError(w, "invalid_request", "Token requests must use form encoding", http.StatusUnsupportedMediaType)
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
		return
	}
	grantType := r.PostFormValue("grant_type")
	switch grantType {
	case "authorization_code":
		code := r.PostFormValue("code")
		rec, err := s.app.OAuth.ConsumeAuthorizationCode(digest(code), s.now())
		verifier := r.PostFormValue("code_verifier")
		if err != nil || rec == nil ||
			rec.ClientID != r.PostFormValue("client_id") ||
			rec.RedirectURI != r.PostFormValue("redirect_uri") ||
			!validPKCE(verifier, rec.CodeChallenge) {
			oauthError(w, "invalid_grant", "Authorization code is invalid or expired", http.StatusBadRequest)
			return
		}
		s.issueTokens(w, rec.GrantID, rec.Scopes)
	case "refresh_token":
		refreshToken := r.PostFormValue("refresh_token")
		nextAccess := "access_" + s.tokens()
		nextRefresh := "refresh_" + s.tokens()
		now := s.now()
		rec, err := s.app.OAuth.RotateRefreshToken(digest(refreshToken), r.PostFormValue("client_id"), store.TokenPair{
			AccessDigest:     digest(nextAccess),
			AccessExpiresAt:  now + accessTokenTTL,
			RefreshDigest:    digest(nextRefresh),
			RefreshExpiresAt: now + refreshTokenTTL,
		}, now)
		if err != nil || rec == nil {
			oauthError(w, "invalid_grant", "Refresh token is invalid or expired", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token":  nextAccess,
			"token_type":    "Bearer",
			"expires_in":    accessTokenTTL,
			"refresh_token": nextRefresh,
			"scope":         strings.Join(rec.Scopes, " "),
		})
	default:
		oauthError(w, "unsupported_grant_type", "Unsupported grant type", http.StatusBadRequest)
	}
}

func (s *RemoteServer) issueTokens(w http.ResponseWriter, grantID string, scopes []string) {
	access := "access_" + s.tokens()
	refresh := "refresh_" + s.tokens()
	now := s.now()
	err := s.app.OAuth.StoreTokens(store.TokenPair{
		AccessDigest:     digest(access),
		AccessExpiresAt:  now + accessTokenTTL,
		GrantID:          grantID,
		RefreshDigest:    digest(refresh),
		RefreshExpiresAt: now + refreshTokenTTL,
	})
	if err != nil {
		safeError(w, "internal", "Failed to store tokens", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    accessTokenTTL,
		"refresh_token": refresh,
		"scope":         strings.Join(scopes, " "),
	})
}

func (s *RemoteServer) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if !isForm(r) {
		oauthError(w, "invalid_request", "Revocation requests must use form encoding", http.StatusUnsupportedMediaType)
		return
	}
	_ = r.ParseForm()
	token := r.PostFormValue("token")
	if token != "" {
		_, _ = s.app.OAuth.RevokeGrantByToken(digest(token), s.now())
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func (s *RemoteServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Origin check
	origin := r.Header.Get("Origin")
	if origin != "" && origin != urlOrigin(s.cfg.PublicURL) {
		safeError(w, "forbidden", "Cross-origin requests are not allowed", http.StatusForbidden)
		return
	}

	// Method check
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		safeError(w, "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Size check
	if r.ContentLength > maxMCPBodyBytes {
		safeError(w, "request_too_large", "Request body is too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Authenticate bearer token
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || token == "" {
		s.unauthorized(w)
		return
	}
	cred, err := s.app.OAuth.FindAccessToken(digest(token))
	if err != nil || cred == nil || cred.Revoked || cred.ExpiresAt <= s.now() {
		s.unauthorized(w)
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusOK)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxMCPBodyBytes+1))
	if err != nil || len(body) > maxMCPBodyBytes {
		safeError(w, "request_too_large", "Request body is too large", http.StatusRequestEntityTooLarge)
		return
	}

	tools := BuildTools(s.app, ToolOptions{
		CanRead:    slices.Contains(cred.Scopes, "mcp:read"),
		CanSend:    slices.Contains(cred.Scopes, "mcp:send"),
		Authorizer: RemoteAuthorizer{},
		IsRemote:   true,
	})
	mcpServer := NewServer("telesend", version.Version, "Send Telegram bot messages through MCP.", s.endpoint("/icon.png"), tools)
	out := mcpServer.Handle(r.Context(), body)
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
}

func (s *RemoteServer) unauthorized(w http.ResponseWriter) {
	meta := s.endpoint("/.well-known/oauth-protected-resource/mcp")
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_token", resource_metadata="%s"`, meta))
	safeError(w, "unauthorized", "Invalid or expired access token", http.StatusUnauthorized)
}

func parseScopes(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return append([]string(nil), supportedScopes...)
	}
	fields := strings.Fields(val)
	for _, f := range fields {
		if !slices.Contains(supportedScopes, f) {
			return nil
		}
	}
	return slices.Compact(fields)
}

func isForm(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/x-www-form-urlencoded")
}

func isBase64URL(s string, min, max int) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func validPKCE(verifier, challenge string) bool {
	if !isBase64URL(verifier, 43, 128) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	actual := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(challenge)) == 1
}

func validRedirectURI(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost") {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(jsonx.Marshal(v))
}

func oauthError(w http.ResponseWriter, code, desc string, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, map[string]any{"error": code, "error_description": desc})
}

func safeError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

const consentHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authorize Telesend</title>
    <link rel="icon" type="image/png" href="/icon.png">
    <link rel="apple-touch-icon" href="/icon.png">
    <style>
      :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
      * { box-sizing: border-box; }
      body { min-height: 100vh; margin: 0; display: grid; place-items: center; padding: 1.5rem 1.5rem 7rem; background: #f6f8fc; color: #172033; }
      main { width: min(100%, 26rem); padding: 2rem; background: #fff; border: 1px solid #dfe5ef; border-radius: 1rem; box-shadow: 0 1rem 2.5rem rgb(30 47 80 / 10%); }
      .app-icon-wrap { display: flex; justify-content: center; align-items: center; margin-bottom: 1.25rem; }
      .app-icon { width: 5.25rem; height: 5.25rem; border-radius: 1.125rem; object-fit: contain; box-shadow: 0 0.5rem 1.25rem rgb(30 47 80 / 12%); border: 1px solid #dfe5ef; }
      h1 { margin: 0; font-size: 1.375rem; line-height: 1.35; letter-spacing: -0.02em; text-align: center; }
      p { margin: 0.625rem 0 1.5rem; color: #5a667c; font-size: 0.9375rem; line-height: 1.5; text-align: center; }
      form { display: grid; gap: 1rem; }
      label { display: grid; gap: 0.5rem; color: #35425a; font-size: 0.875rem; font-weight: 600; }
      input { width: 100%; min-height: 2.75rem; padding: 0.625rem 0.75rem; border: 1px solid #c9d2e1; border-radius: 0.625rem; color: inherit; font: inherit; outline: none; }
      input:focus { border-color: #2563eb; box-shadow: 0 0 0 3px rgb(37 99 235 / 15%); }
      button { min-height: 2.75rem; border: 0; border-radius: 0.625rem; background: #408EC7; color: #fff; cursor: pointer; font: inherit; font-weight: 650; }
      button:hover { background: #1d4ed8; }
    </style>
  </head>
  <body>
    <main>
      <div class="app-icon-wrap"><img src="/icon.png" alt="Telesend" class="app-icon" width="84" height="84"></div>
      <h1>Authorize {{CLIENT_NAME}}</h1>
      <p>Requested access: {{SCOPES}}</p>
      <form method="post" action="/authorize">
        <input type="hidden" name="response_type" value="code">
        <input type="hidden" name="client_id" value="{{CLIENT_ID}}">
        <input type="hidden" name="redirect_uri" value="{{REDIRECT_URI}}">
        <input type="hidden" name="scope" value="{{SCOPE_VAL}}">
        <input type="hidden" name="state" value="{{STATE}}">
        <input type="hidden" name="code_challenge" value="{{CHALLENGE}}">
        <input type="hidden" name="code_challenge_method" value="S256">
        <label>Owner password <input type="password" name="password" required autocomplete="current-password"></label>
        <button type="submit">Authorize</button>
      </form>
    </main>
  </body>
</html>`

func urlOrigin(u *url.URL) string {
    return u.Scheme + "://" + u.Host
}
