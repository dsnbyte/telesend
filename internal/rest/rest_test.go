package rest_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dsnbyte/telesend/internal/rest"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/testutil"
)

func TestRESTServer(t *testing.T) {
	a, tg := testutil.NewApp(t)
	verifier, err := secret.NewVerifier("", "api-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := rest.NewServer(a, verifier)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := ts.Client()

	req := func(method, path string, body []byte, ct string) *http.Response {
		r, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
		r.Header.Set("x-api-key", "api-secret-key")
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return res
	}

	// 1. Unauthenticated requests
	unauth, _ := http.Get(ts.URL + "/health")
	if unauth.StatusCode != 401 {
		t.Fatalf("expected 401: %d", unauth.StatusCode)
	}

	// 2. Health check
	hRes := req(http.MethodGet, "/health", nil, "")
	if hRes.StatusCode != 200 {
		t.Fatalf("health status: %d", hRes.StatusCode)
	}

	// 3. Bot lifecycle
	addBotRes := req(http.MethodPost, "/bots", []byte(`{"token":"first_token"}`), "application/json")
	if addBotRes.StatusCode != 201 {
		t.Fatalf("add bot: %d", addBotRes.StatusCode)
	}
	// Add second bot
	addBotRes2 := req(http.MethodPost, "/bots", []byte(`{"token":"two_token"}`), "application/json")
	if addBotRes2.StatusCode != 201 {
		t.Fatalf("add second bot: %d", addBotRes2.StatusCode)
	}

	// List bots
	listBotsRes := req(http.MethodGet, "/bots", nil, "")
	if listBotsRes.StatusCode != 200 {
		t.Fatalf("list bots: %d", listBotsRes.StatusCode)
	}
	var botsData struct {
		Data []store.Bot `json:"data"`
	}
	json.NewDecoder(listBotsRes.Body).Decode(&botsData)
	if len(botsData.Data) != 2 || !botsData.Data[0].IsDefault {
		t.Fatalf("bots list: %+v", botsData)
	}

	// Set default
	setDefaultRes := req(http.MethodPost, "/bots/other_bot/default", nil, "")
	if setDefaultRes.StatusCode != 200 {
		t.Fatalf("set default: %d", setDefaultRes.StatusCode)
	}

	// Delete non-default
	delBotRes := req(http.MethodDelete, "/bots/default_bot", nil, "")
	if delBotRes.StatusCode != 200 {
		t.Fatalf("delete bot: %d", delBotRes.StatusCode)
	}

	// 4. Alias lifecycle
	addAliasRes := req(http.MethodPost, "/aliases", []byte(`{"name":"ops","chatId":"-100123","messageThreadId":10}`), "application/json")
	if addAliasRes.StatusCode != 201 {
		t.Fatalf("add alias: %d", addAliasRes.StatusCode)
	}

	patchAliasRes := req(http.MethodPatch, "/aliases/ops", []byte(`{"name":"ops-new","messageThreadId":20}`), "application/json")
	if patchAliasRes.StatusCode != 200 {
		t.Fatalf("patch alias: %d", patchAliasRes.StatusCode)
	}

	delAliasRes := req(http.MethodDelete, "/aliases/ops-new", nil, "")
	if delAliasRes.StatusCode != 200 {
		t.Fatalf("delete alias: %d", delAliasRes.StatusCode)
	}

	// Recreate alias ops
	_ = req(http.MethodPost, "/aliases", []byte(`{"name":"ops","chatId":"-100123"}`), "application/json")

	// 5. Send message via JSON
	msgRes := req(http.MethodPost, "/messages/text", []byte(`{"to":"ops","payload":{"text":"hello from rest"}}`), "application/json")
	if msgRes.StatusCode != 200 {
		t.Fatalf("send msg: %d", msgRes.StatusCode)
	}
	if len(tg.Calls()) == 0 {
		t.Fatal("expected delivery to telegram")
	}

	// Send message directly to chat ID
	chatRes := req(http.MethodPost, "/messages/text", []byte(`{"to":"-100999","payload":{"text":"direct chat id test"}}`), "application/json")
	if chatRes.StatusCode != 200 {
		t.Fatalf("send direct chat id: %d", chatRes.StatusCode)
	}

	// Reject server paths in JSON
	pathRes := req(http.MethodPost, "/messages/document", []byte(`{"to":"ops","payload":{"document":{"source":"path","value":"/etc/passwd"}}}`), "application/json")
	if pathRes.StatusCode != 400 {
		t.Fatalf("expected 400 for server path: %d", pathRes.StatusCode)
	}

	// 6. Send message via multipart upload
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("to", "ops")
	mw.WriteField("payload", `{"caption":"Photo caption"}`)
	part, _ := mw.CreateFormFile("photo", "pic.png")
	part.Write([]byte("image-bytes"))
	mw.Close()

	uploadRes := req(http.MethodPost, "/messages/photo", b.Bytes(), mw.FormDataContentType())
	if uploadRes.StatusCode != 200 {
		body, _ := io.ReadAll(uploadRes.Body)
		t.Fatalf("upload photo: %d %s", uploadRes.StatusCode, string(body))
	}
}
