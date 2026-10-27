package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsnbyte/telesend/internal/mcp"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/testutil"
)

func TestFilePolicy(t *testing.T) {
	cwd := t.TempDir()
	configured := t.TempDir()
	cliRoot := filepath.Join(cwd, "cli")
	os.MkdirAll(cliRoot, 0o755)

	os.WriteFile(filepath.Join(cwd, "a.pdf"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(configured, "b.pdf"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(cliRoot, "c.pdf"), []byte("c"), 0o644)

	policy, err := mcp.NewFilePolicy(mcp.FilePolicyOptions{
		CWD:             cwd,
		ConfiguredRoots: []string{configured},
		CLIRoots:        []string{"cli"},
		Include:         []string{"included.ts"},
		Exclude:         []string{"excluded.ts"},
		ProtectedPaths:  []string{filepath.Join(cwd, "protected.db")},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{filepath.Join(cwd, "a.pdf"), filepath.Join(configured, "b.pdf"), filepath.Join(cliRoot, "c.pdf")} {
		if _, err := policy.Authorize(p); err != nil {
			t.Fatalf("expected %s allowed: %v", p, err)
		}
	}

	// Outside file
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	os.WriteFile(outside, []byte("s"), 0o644)
	if _, err := policy.Authorize(outside); err == nil {
		t.Fatal("expected outside error")
	}

	// Symlink escape
	link := filepath.Join(cwd, "escape.pdf")
	os.Symlink(outside, link)
	if _, err := policy.Authorize(link); err == nil {
		t.Fatal("expected symlink escape error")
	}

	// Protected, exclude, default, include
	os.WriteFile(filepath.Join(cwd, ".env"), []byte("secret"), 0o644)
	os.WriteFile(filepath.Join(cwd, "source.go"), []byte("code"), 0o644)
	os.WriteFile(filepath.Join(cwd, "included.ts"), []byte("inc"), 0o644)
	os.WriteFile(filepath.Join(cwd, "excluded.ts"), []byte("exc"), 0o644)

	if _, err := policy.Authorize(filepath.Join(cwd, ".env")); err == nil {
		t.Fatal("expected protected error for .env")
	}
	if _, err := policy.Authorize(filepath.Join(cwd, "source.go")); err == nil {
		t.Fatal("expected default exclusion for source.go")
	}
	if _, err := policy.Authorize(filepath.Join(cwd, "excluded.ts")); err == nil {
		t.Fatal("expected exclude pattern error")
	}
	if _, err := policy.Authorize(filepath.Join(cwd, "included.ts")); err != nil {
		t.Fatal("expected included.ts allowed:", err)
	}
}

func TestStdioMCPServer(t *testing.T) {
	a, _ := testutil.WithBot(t)
	cwd := t.TempDir()
	policy, _ := mcp.NewFilePolicy(mcp.FilePolicyOptions{CWD: cwd})
	server := mcp.NewStdioServer(a, policy)

	// initialize
	initResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
	if !strings.Contains(string(initResp), `"protocolVersion":"2025-11-25"`) || !strings.Contains(string(initResp), `"telesend"`) {
		t.Fatalf("init: %s", string(initResp))
	}

	// tools/list
	listResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if !strings.Contains(string(listResp), "list_aliases") || !strings.Contains(string(listResp), "send_text") {
		t.Fatalf("tools/list: %s", string(listResp))
	}

	// call list_bots
	callResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_bots"}}`))
	if !strings.Contains(string(callResp), "default_bot") {
		t.Fatalf("list_bots call: %s", string(callResp))
	}

	// call list_aliases with and without filter
	a.Aliases.Create(store.AliasInput{Name: "ops", ChatID: "-100123"})
	a.Aliases.Create(store.AliasInput{Name: "alice", ChatID: "12345"})

	allAliasResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"list_aliases"}}`))
	if !strings.Contains(string(allAliasResp), "ops") || !strings.Contains(string(allAliasResp), "alice") {
		t.Fatalf("list_aliases all: %s", string(allAliasResp))
	}

	groupAliasResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"list_aliases","arguments":{"type":"group"}}}`))
	if !strings.Contains(string(groupAliasResp), "ops") || strings.Contains(string(groupAliasResp), "alice") {
		t.Fatalf("list_aliases type=group: %s", string(groupAliasResp))
	}

	privAliasResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"list_aliases","arguments":{"type":"private"}}}`))
	if !strings.Contains(string(privAliasResp), "alice") || strings.Contains(string(privAliasResp), "ops") {
		t.Fatalf("list_aliases type=private: %s", string(privAliasResp))
	}

	filterGroupResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"list_aliases","arguments":{"filter":"group"}}}`))
	if !strings.Contains(string(filterGroupResp), "ops") || strings.Contains(string(filterGroupResp), "alice") {
		t.Fatalf("list_aliases filter=group: %s", string(filterGroupResp))
	}

	invalidAliasResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"list_aliases","arguments":{"type":"invalid"}}}`))
	if !strings.Contains(string(invalidAliasResp), `"isError":true`) {
		t.Fatalf("expected error for invalid type filter: %s", string(invalidAliasResp))
	}

	// call send_text
	sendResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"send_text","arguments":{"to":"1","payload":{"text":"hi"}}}}`))
	if strings.Contains(string(sendResp), `"isError":true`) || !strings.Contains(string(sendResp), "messageId") {
		t.Fatalf("send_text call: %s", string(sendResp))
	}

	// call send_text with inline #<thread_id>
	inlineResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"send_text","arguments":{"to":"-100123#42","payload":{"text":"hi thread"}}}}`))
	if strings.Contains(string(inlineResp), `"isError":true`) || !strings.Contains(string(inlineResp), `"messageThreadId":42`) {
		t.Fatalf("send_text inline thread: %s", string(inlineResp))
	}

	// call send_text with inline #<thread_id> on private chat (thread ignored)
	privResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"send_text","arguments":{"to":"123456#42","payload":{"text":"hi private"}}}}`))
	if strings.Contains(string(privResp), `"isError":true`) || strings.Contains(string(privResp), "messageThreadId") {
		t.Fatalf("send_text private inline thread ignored: %s", string(privResp))
	}

	// call get_telegram_parameter_doc
	docResp := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_telegram_parameter_doc","arguments":{"method":"sendMessage"}}}`))
	if strings.Contains(string(docResp), `"isError":true`) || !strings.Contains(string(docResp), "sendMessage") {
		t.Fatalf("doc call: %s", string(docResp))
	}
}

func TestRemoteMCPServerAndOAuth(t *testing.T) {
	a, tg := testutil.WithBot(t)
	publicURL, _ := url.Parse("https://mcp.example.com")
	verifier, _ := secret.NewVerifier("", "owner_secret")

	now := int64(1000)
	srv, err := mcp.NewRemoteServer(a, mcp.RemoteConfig{
		PublicURL:     publicURL,
		OwnerVerifier: verifier,
		Now:           func() int64 { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := ts.Client()

	// 1. Discovery
	res, err := client.Get(ts.URL + "/.well-known/oauth-protected-resource/mcp")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("discovery: %v %v", res, err)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "https://mcp.example.com/mcp") {
		t.Fatalf("discovery body: %s", string(body))
	}

	// 2. Icon
	iconRes, err := client.Get(ts.URL + "/icon.png")
	if err != nil || iconRes.StatusCode != 200 || iconRes.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("icon: %v %v", iconRes, err)
	}

	// 3. Register client
	regBody := `{"redirect_uris":["https://app.example.com/cb"],"client_name":"Agent"}`
	regRes, err := client.Post(ts.URL+"/register", "application/json", strings.NewReader(regBody))
	if err != nil || regRes.StatusCode != 201 {
		t.Fatalf("register: %v %v", regRes, err)
	}
	var regData struct {
		ClientID string `json:"client_id"`
	}
	json.NewDecoder(regRes.Body).Decode(&regData)
	if !strings.HasPrefix(regData.ClientID, "client_") {
		t.Fatalf("client id: %+v", regData)
	}

	// 4. Authorize (GET then POST)
	verifierStr := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challengeStr := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	authURL := ts.URL + "/authorize?client_id=" + regData.ClientID +
		"&redirect_uri=https://app.example.com/cb&response_type=code&code_challenge=" + challengeStr +
		"&code_challenge_method=S256&scope=mcp:read%20mcp:send&state=xyz"
	getAuthRes, err := client.Get(authURL)
	if err != nil || getAuthRes.StatusCode != 200 {
		t.Fatalf("get authorize: %v %v", getAuthRes, err)
	}

	// POST authorize with owner password
	postForm := url.Values{
		"client_id":             {regData.ClientID},
		"redirect_uri":          {"https://app.example.com/cb"},
		"response_type":         {"code"},
		"code_challenge":        {challengeStr},
		"code_challenge_method": {"S256"},
		"scope":                 {"mcp:read mcp:send"},
		"state":                 {"xyz"},
		"password":              {"owner_secret"},
	}
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	postAuthRes, err := noRedirectClient.PostForm(ts.URL+"/authorize", postForm)
	if err != nil || postAuthRes.StatusCode != 302 {
		t.Fatalf("post authorize: %v %v", postAuthRes, err)
	}
	loc, _ := postAuthRes.Location()
	code := loc.Query().Get("code")
	if !strings.HasPrefix(code, "code_") || loc.Query().Get("state") != "xyz" {
		t.Fatalf("redirect loc: %v", loc)
	}

	// 5. Exchange code for tokens
	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regData.ClientID},
		"redirect_uri":  {"https://app.example.com/cb"},
		"code":          {code},
		"code_verifier": {verifierStr},
	}
	tokenRes, err := client.PostForm(ts.URL+"/token", tokenForm)
	if err != nil || tokenRes.StatusCode != 200 {
		t.Fatalf("token: %v %v", tokenRes, err)
	}
	var tokenData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	json.NewDecoder(tokenRes.Body).Decode(&tokenData)
	if !strings.HasPrefix(tokenData.AccessToken, "access_") || !strings.HasPrefix(tokenData.RefreshToken, "refresh_") {
		t.Fatalf("token data: %+v", tokenData)
	}

	// 6. Access /mcp with bearer token
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"send_text","arguments":{"to":"1","payload":{"text":"remote test"}}}}`)))
	req.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	mcpRes, err := client.Do(req)
	if err != nil || mcpRes.StatusCode != 200 {
		t.Fatalf("mcp call: %v %v", mcpRes, err)
	}
	var callResult struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	json.NewDecoder(mcpRes.Body).Decode(&callResult)
	if callResult.Result.IsError || len(tg.Calls()) == 0 {
		t.Fatalf("expected delivery: %+v", callResult)
	}

	// 7. Revoke token
	revokeForm := url.Values{"token": {tokenData.AccessToken}}
	revRes, err := client.PostForm(ts.URL+"/revoke", revokeForm)
	if err != nil || revRes.StatusCode != 200 {
		t.Fatalf("revoke: %v %v", revRes, err)
	}

	// After revocation, /mcp returns 401
	unauthReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader([]byte(`{}`)))
	unauthReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	unauthRes, err := client.Do(unauthReq)
	if err != nil || unauthRes.StatusCode != 401 {
		t.Fatalf("expected 401 after revocation: %v %v", unauthRes, err)
	}
}

func TestMethodDocsBotAPI103(t *testing.T) {
	// sendMessage should have ephemeral_message_parameters and allow_paid_broadcast
	doc, ok := mcp.MethodDocFor("sendMessage")
	if !ok {
		t.Fatal("missing doc for sendMessage")
	}
	hasEphemeral := false
	hasPaidBroadcast := false
	for _, p := range doc.AdvancedParams {
		if p.Name == "ephemeral_message_parameters" {
			hasEphemeral = true
		}
		if p.Name == "allow_paid_broadcast" {
			hasPaidBroadcast = true
		}
	}
	if !hasEphemeral || !hasPaidBroadcast {
		t.Fatalf("sendMessage missing 10.3 parameters: ephemeral=%v, paid_broadcast=%v", hasEphemeral, hasPaidBroadcast)
	}

	// sendMessageDraft should have can_stop, keep_on_stop, and NOT reply_parameters
	draftDoc, ok := mcp.MethodDocFor("sendMessageDraft")
	if !ok {
		t.Fatal("missing doc for sendMessageDraft")
	}
	hasCanStop := false
	hasKeepOnStop := false
	hasReplyParams := false
	for _, p := range draftDoc.AdvancedParams {
		if p.Name == "can_stop" {
			hasCanStop = true
		}
		if p.Name == "keep_on_stop" {
			hasKeepOnStop = true
		}
		if p.Name == "reply_parameters" {
			hasReplyParams = true
		}
	}
	if !hasCanStop || !hasKeepOnStop || hasReplyParams {
		t.Fatalf("sendMessageDraft incorrect params: can_stop=%v, keep_on_stop=%v, reply_parameters=%v", hasCanStop, hasKeepOnStop, hasReplyParams)
	}

	// sendPoll should have correct_option_ids (not correct_option_id) and allows_revoting
	pollDoc, ok := mcp.MethodDocFor("sendPoll")
	if !ok {
		t.Fatal("missing doc for sendPoll")
	}
	hasCorrectOptionIDs := false
	hasOldCorrectOptionID := false
	hasAllowsRevoting := false
	for _, p := range pollDoc.AdvancedParams {
		if p.Name == "correct_option_ids" {
			hasCorrectOptionIDs = true
		}
		if p.Name == "correct_option_id" {
			hasOldCorrectOptionID = true
		}
		if p.Name == "allows_revoting" {
			hasAllowsRevoting = true
		}
	}
	if !hasCorrectOptionIDs || hasOldCorrectOptionID || !hasAllowsRevoting {
		t.Fatalf("sendPoll incorrect params: correct_option_ids=%v, old=%v, allows_revoting=%v", hasCorrectOptionIDs, hasOldCorrectOptionID, hasAllowsRevoting)
	}

	// sendPhoto should not have duplicate show_caption_above_media
	photoDoc, ok := mcp.MethodDocFor("sendPhoto")
	if !ok {
		t.Fatal("missing doc for sendPhoto")
	}
	captionAboveCount := 0
	for _, p := range photoDoc.AdvancedParams {
		if p.Name == "show_caption_above_media" {
			captionAboveCount++
		}
	}
	if captionAboveCount != 1 {
		t.Fatalf("sendPhoto show_caption_above_media count = %d, expected 1", captionAboveCount)
	}

	// sendAudio should NOT have show_caption_above_media
	audioDoc, ok := mcp.MethodDocFor("sendAudio")
	if !ok {
		t.Fatal("missing doc for sendAudio")
	}
	for _, p := range audioDoc.AdvancedParams {
		if p.Name == "show_caption_above_media" {
			t.Fatal("sendAudio should not have show_caption_above_media")
		}
	}

	// sendChecklist should have business_connection_id with Required == true
	checklistDoc, ok := mcp.MethodDocFor("sendChecklist")
	if !ok {
		t.Fatal("missing doc for sendChecklist")
	}
	for _, p := range checklistDoc.AdvancedParams {
		if p.Name == "business_connection_id" && !p.Required {
			t.Fatal("sendChecklist business_connection_id should be required")
		}
	}
}

