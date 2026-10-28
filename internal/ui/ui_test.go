package ui_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/testutil"
	"github.com/dsnbyte/telesend/internal/ui"
)

func setupTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	a, _ := testutil.NewApp(t)
	v, err := secret.NewVerifier("", "test_ui_password")
	if err != nil {
		t.Fatal(err)
	}
	server, err := ui.NewServer(a, v)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		a.Close()
	})

	// Client with cookie jar
	client := ts.Client()
	return ts, client
}

func TestStaticAssets(t *testing.T) {
	ts, client := setupTestServer(t)

	// 1. Root index.html
	res, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for /, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	htmlContent := string(body)
	if !strings.Contains(htmlContent, "<span class=\"brand-title\">Telesend</span>") {
		t.Fatal("expected brand title 'Telesend' with uppercase first letter")
	}
	if !strings.Contains(htmlContent, "width=\"28\" height=\"28\"") {
		t.Fatal("expected brand icon logo enlarged to width 28 height 28")
	}
	if strings.Contains(htmlContent, "section-sub") {
		t.Fatal("expected tagline/description below page titles to be removed")
	}
	if strings.Contains(htmlContent, "Telesend self-hosted Telegram gateway") {
		t.Fatal("expected footer 'Telesend self-hosted Telegram gateway' to be removed")
	}
	if !strings.Contains(htmlContent, "modal-add-bot") || !strings.Contains(htmlContent, "modal-add-alias") {
		t.Fatal("expected modal dialogs for add bot and add alias")
	}
	if !strings.Contains(htmlContent, "modal-confirm") {
		t.Fatal("expected reusable confirm modal dialog")
	}
	if !strings.Contains(htmlContent, "tab-btn-detail-preview") || !strings.Contains(htmlContent, "tab-btn-detail-payload") {
		t.Fatal("expected Preview and Payload tabs in Delivery Details modal")
	}
	if !strings.Contains(htmlContent, "hist-detail-preview") {
		t.Fatal("expected message preview container in Delivery Details modal")
	}

	// 2. CSS
	resCSS, err := client.Get(ts.URL + "/app.css")
	if err != nil || resCSS.StatusCode != 200 {
		t.Fatalf("expected 200 for /app.css, got %v", resCSS)
	}
	cssBody, _ := io.ReadAll(resCSS.Body)
	resCSS.Body.Close()
	cssContent := string(cssBody)
	fontExpected := `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji"`
	if !strings.Contains(cssContent, fontExpected) {
		t.Fatalf("expected CSS font-family to contain %s", fontExpected)
	}
	if !strings.Contains(cssContent, "max-width: 930px") {
		t.Fatal("expected app-container max-width 930px (1.5x)")
	}
	if !strings.Contains(cssContent, ".btn-action-icon") {
		t.Fatal("expected .btn-action-icon CSS rule")
	}
	if !strings.Contains(cssContent, ".tg-bubble-container") || !strings.Contains(cssContent, ".tg-bubble") {
		t.Fatal("expected Telegram-styled chat bubble CSS rules")
	}

	// 3. JS
	resJS, err := client.Get(ts.URL + "/app.js")
	if err != nil || resJS.StatusCode != 200 {
		t.Fatalf("expected 200 for /app.js, got %v", resJS)
	}
	jsBody, _ := io.ReadAll(resJS.Body)
	resJS.Body.Close()
	jsContent := string(jsBody)
	if !strings.Contains(jsContent, "showConfirmDialog") {
		t.Fatal("expected showConfirmDialog centralized confirmation function in app.js")
	}
	if !strings.Contains(jsContent, "renderTelegramPreview") {
		t.Fatal("expected renderTelegramPreview in app.js")
	}
	if !strings.Contains(jsContent, "ICONS") {
		t.Fatal("expected ICONS object with reusable SVG icons in app.js")
	}

	// 4. Icon
	resIcon, err := client.Get(ts.URL + "/icon.png")
	if err != nil || resIcon.StatusCode != 200 {
		t.Fatalf("expected 200 for /icon.png, got %v", resIcon)
	}
	resIcon.Body.Close()
}

func TestAuthLifecycleAndEndpoints(t *testing.T) {
	ts, _ := setupTestServer(t)
	// Create jar to track cookies across requests
	jar := make(map[string]*http.Cookie)
	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for _, c := range jar {
			req.AddCookie(c)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range res.Cookies() {
			if c.MaxAge < 0 {
				delete(jar, c.Name)
			} else {
				jar[c.Name] = c
			}
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, data
	}

	// 1. Initial auth status: initialized and not authenticated
	res, data := doReq(http.MethodGet, "/api/ui/auth/status", nil)
	if res.StatusCode != 200 {
		t.Fatalf("status code %d", res.StatusCode)
	}
	var st map[string]any
	_ = json.Unmarshal(data, &st)
	if st["initialized"] != true || st["authenticated"] != false {
		t.Fatalf("expected initialized and unauthenticated: %+v", st)
	}

	// 2. Accessing protected endpoint before login fails
	res, _ = doReq(http.MethodGet, "/api/ui/bots", nil)
	if res.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}

	// 3. Login with wrong password fails
	res, _ = doReq(http.MethodPost, "/api/ui/auth/login", map[string]string{
		"password": "wrong_password",
	})
	if res.StatusCode != 401 {
		t.Fatalf("expected 401 for wrong password, got %d", res.StatusCode)
	}

	// 4. Login with correct password succeeds
	res, _ = doReq(http.MethodPost, "/api/ui/auth/login", map[string]string{
		"password": "test_ui_password",
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for login, got %d", res.StatusCode)
	}
	if jar["telesend_session"] == nil {
		t.Fatal("expected session cookie set after login")
	}

	// 5. Auth status is now authenticated
	res, data = doReq(http.MethodGet, "/api/ui/auth/status", nil)
	_ = json.Unmarshal(data, &st)
	if st["initialized"] != true || st["authenticated"] != true {
		t.Fatalf("expected authenticated: %+v", st)
	}

	// 6. Protected endpoint succeeds now
	res, _ = doReq(http.MethodGet, "/api/ui/bots", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for /bots, got %d", res.StatusCode)
	}

	// 7. Logout clears session
	res, _ = doReq(http.MethodPost, "/api/ui/auth/logout", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for logout, got %d", res.StatusCode)
	}
	if jar["telesend_session"] != nil {
		t.Fatal("expected session cookie to be removed after logout")
	}

	// 8. Protected endpoint fails again after logout
	res, _ = doReq(http.MethodGet, "/api/ui/bots", nil)
	if res.StatusCode != 401 {
		t.Fatalf("expected 401 after logout, got %d", res.StatusCode)
	}

	// 9. Hashed password verifier support
	rawHash, err := secret.Hash("hashed_master_pw")
	if err != nil {
		t.Fatal(err)
	}
	vHashed, err := secret.NewVerifier(secret.Encode(rawHash), "")
	if err != nil {
		t.Fatal(err)
	}
	aHashed, _ := testutil.NewApp(t)
	defer aHashed.Close()
	srvHashed, err := ui.NewServer(aHashed, vHashed)
	if err != nil {
		t.Fatal(err)
	}
	tsHashed := httptest.NewServer(srvHashed)
	defer tsHashed.Close()

	// Try wrong password on hashed server
	bWrong, _ := json.Marshal(map[string]string{"password": "wrong"})
	resHashedWrong, err := http.Post(tsHashed.URL+"/api/ui/auth/login", "application/json", bytes.NewReader(bWrong))
	if err != nil || resHashedWrong.StatusCode != 401 {
		t.Fatalf("expected 401 for wrong password on hashed server: %v, code %d", err, resHashedWrong.StatusCode)
	}

	// Try correct password on hashed server
	bRight, _ := json.Marshal(map[string]string{"password": "hashed_master_pw"})
	resHashedRight, err := http.Post(tsHashed.URL+"/api/ui/auth/login", "application/json", bytes.NewReader(bRight))
	if err != nil || resHashedRight.StatusCode != 200 {
		t.Fatalf("expected 200 for correct password on hashed server: %v, code %d", err, resHashedRight.StatusCode)
	}

	// 10. Nil verifier returns configuration error
	_, err = ui.NewServer(aHashed, nil)
	if err == nil {
		t.Fatal("expected error with nil verifier")
	}
}

func TestBotsAndAliasesManagement(t *testing.T) {
	ts, _ := setupTestServer(t)
	jar := make(map[string]*http.Cookie)
	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for _, c := range jar {
			req.AddCookie(c)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range res.Cookies() {
			jar[c.Name] = c
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, data
	}

	// Login
	doReq(http.MethodPost, "/api/ui/auth/login", map[string]string{
		"password": "test_ui_password",
	})

	// 1. Add bot
	res, data := doReq(http.MethodPost, "/api/ui/bots", map[string]string{
		"token": "bot_token",
	})
	if res.StatusCode != 201 {
		t.Fatalf("expected 201 adding bot, got %d %s", res.StatusCode, string(data))
	}
	var botRes struct {
		Data struct {
			Username  string `json:"username"`
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &botRes)
	if botRes.Data.Username != "default_bot" || !botRes.Data.IsDefault {
		t.Fatalf("bot res mismatch: %+v", botRes)
	}

	// 2. Add second bot
	doReq(http.MethodPost, "/api/ui/bots", map[string]string{
		"token": "two_token",
	})

	// 3. List bots
	res, data = doReq(http.MethodGet, "/api/ui/bots", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 listing bots, got %d", res.StatusCode)
	}
	var listRes struct {
		Data []struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &listRes)
	if len(listRes.Data) != 2 {
		t.Fatalf("expected 2 bots, got %d", len(listRes.Data))
	}

	// 4. Update bot
	res, data = doReq(http.MethodPatch, "/api/ui/bots/default_bot", map[string]string{
		"name": "Updated Primary Bot",
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 updating bot, got %d %s", res.StatusCode, string(data))
	}

	// 5. Set default bot
	res, _ = doReq(http.MethodPost, "/api/ui/bots/other_bot/default", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 setting default bot, got %d", res.StatusCode)
	}

	// 6. Delete bot
	res, _ = doReq(http.MethodDelete, "/api/ui/bots/default_bot", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 deleting non-default bot, got %d", res.StatusCode)
	}

	// 7. Add Alias
	thread := 42
	res, data = doReq(http.MethodPost, "/api/ui/aliases", map[string]any{
		"name":            "ops-team",
		"chatId":          "-100123456789",
		"messageThreadId": thread,
	})
	if res.StatusCode != 201 {
		t.Fatalf("expected 201 adding alias, got %d %s", res.StatusCode, string(data))
	}

	// 8. Add Private Alias
	doReq(http.MethodPost, "/api/ui/aliases", map[string]any{
		"name":   "alice",
		"chatId": "12345678",
	})

	// 9. List aliases with filter
	res, data = doReq(http.MethodGet, "/api/ui/aliases?filter=group", nil)
	var aliasList struct {
		Data []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &aliasList)
	if len(aliasList.Data) != 1 || aliasList.Data[0].Name != "ops-team" {
		t.Fatalf("expected 1 group alias: %+v", aliasList)
	}

	// 10. Update Alias
	res, _ = doReq(http.MethodPatch, "/api/ui/aliases/ops-team", map[string]any{
		"name":   "ops-renamed",
		"chatId": "-100987654321",
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 updating alias, got %d", res.StatusCode)
	}

	// 11. Delete Alias
	res, _ = doReq(http.MethodDelete, "/api/ui/aliases/alice", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 deleting alias, got %d", res.StatusCode)
	}

	// 12. Hash Tool
	res, data = doReq(http.MethodPost, "/api/ui/hash", map[string]string{
		"type":  "key",
		"value": "my-secret-key",
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for hash tool, got %d", res.StatusCode)
	}
	var hashRes struct {
		EnvLine string `json:"envLine"`
	}
	_ = json.Unmarshal(data, &hashRes)
	if !strings.HasPrefix(hashRes.EnvLine, "TELESEND_API_KEY_HASH=") {
		t.Fatalf("unexpected hash output: %s", hashRes.EnvLine)
	}

	// 12b. Hash Tool with UI password
	res, data = doReq(http.MethodPost, "/api/ui/hash", map[string]string{
		"type":  "ui-password",
		"value": "my-ui-password",
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for ui hash tool, got %d", res.StatusCode)
	}
	var hashUIRes struct {
		EnvLine string `json:"envLine"`
	}
	_ = json.Unmarshal(data, &hashUIRes)
	if !strings.HasPrefix(hashUIRes.EnvLine, "TELESEND_UI_PASSWORD_HASH=") {
		t.Fatalf("unexpected ui hash output: %s", hashUIRes.EnvLine)
	}

	// 13. Server Info
	res, data = doReq(http.MethodGet, "/api/ui/server-info", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for server-info, got %d", res.StatusCode)
	}
	var srvRes struct {
		McpTools []any `json:"mcpTools"`
	}
	_ = json.Unmarshal(data, &srvRes)
	if len(srvRes.McpTools) == 0 {
		t.Fatal("expected mcp tools in server-info")
	}

	// 14. Send Message (JSON)
	res, data = doReq(http.MethodPost, "/api/ui/send", map[string]any{
		"type": "text",
		"to":   "ops-renamed",
		"payload": map[string]any{
			"text": "Hello ops",
		},
	})
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for send, got %d %s", res.StatusCode, string(data))
	}

	// 15. Send Message (Multipart with file)
	var mpBody bytes.Buffer
	writer := multipart.NewWriter(&mpBody)
	_ = writer.WriteField("type", "document")
	_ = writer.WriteField("to", "ops-renamed")
	part, _ := writer.CreateFormFile("document", "test.txt")
	_, _ = part.Write([]byte("file content here"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/ui/send", &mpBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range jar {
		req.AddCookie(c)
	}
	resMP, err := http.DefaultClient.Do(req)
	if err != nil || resMP.StatusCode != 200 {
		t.Fatalf("expected 200 for multipart send, got %v", resMP)
	}
	resMP.Body.Close()

	// 16. History Endpoints
	res, data = doReq(http.MethodGet, "/api/ui/history", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for get history, got %d", res.StatusCode)
	}
	var histRes struct {
		Data          []store.HistoryEntry `json:"data"`
		RetentionDays int                  `json:"retentionDays"`
	}
	_ = json.Unmarshal(data, &histRes)
	if len(histRes.Data) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(histRes.Data))
	}
	if histRes.RetentionDays != 7 {
		t.Fatalf("expected 7 retention days, got %d", histRes.RetentionDays)
	}

	firstID := histRes.Data[0].ID
	res, data = doReq(http.MethodGet, "/api/ui/history/"+strconv.FormatInt(firstID, 10), nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for get single history, got %d", res.StatusCode)
	}

	res, data = doReq(http.MethodDelete, "/api/ui/history/"+strconv.FormatInt(firstID, 10), nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for delete single history, got %d", res.StatusCode)
	}

	secondID := histRes.Data[1].ID
	res, data = doReq(http.MethodDelete, "/api/ui/history/"+strconv.FormatInt(secondID, 10)+"?delete_telegram=true", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for delete history with telegram, got %d: %s", res.StatusCode, string(data))
	}

	res, data = doReq(http.MethodDelete, "/api/ui/history", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for clear history, got %d", res.StatusCode)
	}

	res, data = doReq(http.MethodGet, "/api/ui/history", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 for get history after clear, got %d", res.StatusCode)
	}
	_ = json.Unmarshal(data, &histRes)
	if len(histRes.Data) != 0 {
		t.Fatalf("expected 0 history entries after clear, got %d", len(histRes.Data))
	}
}

func TestHistoryPaginationAndConfig(t *testing.T) {
	ts, _ := setupTestServer(t)
	jar := make(map[string]*http.Cookie)
	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for _, c := range jar {
			req.AddCookie(c)
		}
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range res.Cookies() {
			jar[c.Name] = c
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, data
	}

	// Login
	doReq(http.MethodPost, "/api/ui/auth/login", map[string]string{"password": "test_ui_password"})
	// Register bot & alias
	doReq(http.MethodPost, "/api/ui/bots", map[string]string{"token": "page_bot_token"})
	doReq(http.MethodPost, "/api/ui/aliases", map[string]any{"name": "team", "chatId": "-100777"})

	// Send 5 messages
	for i := 1; i <= 5; i++ {
		doReq(http.MethodPost, "/api/ui/send", map[string]any{
			"type":    "text",
			"to":      "team",
			"payload": map[string]any{"text": fmt.Sprintf("message %d", i)},
		})
	}

	// Fetch page 1 with limit=2
	res, data := doReq(http.MethodGet, "/api/ui/history?limit=2&offset=0", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, string(data))
	}
	var page1 struct {
		Data     []store.HistoryEntry `json:"data"`
		Total    int                  `json:"total"`
		PageSize int                  `json:"pageSize"`
		Offset   int                  `json:"offset"`
		HasMore  bool                 `json:"hasMore"`
	}
	_ = json.Unmarshal(data, &page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 entries on page 1, got %d", len(page1.Data))
	}
	if page1.Total != 5 {
		t.Fatalf("expected total 5, got %d", page1.Total)
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore to be true")
	}
	// Verify newest is first: message 5 should be at index 0
	if page1.Data[0].Summary != "message 5" {
		t.Fatalf("expected newest message ('message 5') first, got %q", page1.Data[0].Summary)
	}

	// Fetch page 2 with limit=2 & offset=2
	res, data = doReq(http.MethodGet, "/api/ui/history?limit=2&offset=2", nil)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, string(data))
	}
	var page2 struct {
		Data    []store.HistoryEntry `json:"data"`
		HasMore bool                 `json:"hasMore"`
	}
	_ = json.Unmarshal(data, &page2)
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 entries on page 2, got %d", len(page2.Data))
	}
	if page2.Data[0].Summary != "message 3" {
		t.Fatalf("expected 'message 3', got %q", page2.Data[0].Summary)
	}
	if !page2.HasMore {
		t.Fatal("expected hasMore to be true on page 2")
	}

	// Fetch page 3 with limit=2 & offset=4 (last remaining 1)
	res, data = doReq(http.MethodGet, "/api/ui/history?limit=2&offset=4", nil)
	var page3 struct {
		Data    []store.HistoryEntry `json:"data"`
		HasMore bool                 `json:"hasMore"`
	}
	_ = json.Unmarshal(data, &page3)
	if len(page3.Data) != 1 {
		t.Fatalf("expected 1 entry on page 3, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Fatal("expected hasMore to be false on page 3")
	}
}
