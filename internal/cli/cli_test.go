package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsnbyte/telesend/internal/cli"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/testutil"
	"github.com/dsnbyte/telesend/internal/version"
)

func run(ctx context.Context, a *testutil.Telegram, appRef any, args []string, in string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	inBuf := strings.NewReader(in)
	ioStreams := cli.IO{Out: &outBuf, Err: &errBuf, In: inBuf}
	code = cli.Run(ctx, args, appRef.(*testutil.AppWrapper).App, ioStreams)
	return outBuf.String(), errBuf.String(), code
}

type testEnv struct {
	wrapper *testutil.AppWrapper
	tg      *testutil.Telegram
}

func setupCLI(t testing.TB) *testEnv {
	t.Helper()
	a, tg := testutil.NewApp(t)
	return &testEnv{
		wrapper: &testutil.AppWrapper{App: a},
		tg:      tg,
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	env := setupCLI(t)
	ctx := context.Background()

	out, _, code := run(ctx, env.tg, env.wrapper, []string{"--version"}, "")
	if code != 0 || strings.TrimSpace(out) != version.Version {
		t.Fatalf("version: %d %q", code, out)
	}

	out, _, code = run(ctx, env.tg, env.wrapper, []string{"help"}, "")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("help: %d %q", code, out)
	}

	_, errOut, code := run(ctx, env.tg, env.wrapper, []string{"invalid_command"}, "")
	if code != 1 || !strings.Contains(errOut, "Unknown command") {
		t.Fatalf("invalid command: %d %q", code, errOut)
	}
}

func TestCLIBotManagement(t *testing.T) {
	env := setupCLI(t)
	ctx := context.Background()

	// 1. Add bot
	out, _, code := run(ctx, env.tg, env.wrapper, []string{"bot", "add"}, "bot_token\n")
	if code != 0 || !strings.Contains(out, "Added bot @default_bot") {
		t.Fatalf("bot add: %d %q", code, out)
	}

	// 2. List bots
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"bot", "list"}, "")
	if code != 0 || !strings.Contains(out, "USERNAME") || !strings.Contains(out, "default_bot") {
		t.Fatalf("bot list: %d %q", code, out)
	}

	// 3. Add second bot
	run(ctx, env.tg, env.wrapper, []string{"bot", "add"}, "two_token\n")

	// 4. Default bot
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"bot", "default", "other_bot"}, "")
	if code != 0 || !strings.Contains(out, "Default bot set to @other_bot") {
		t.Fatalf("bot default: %d %q", code, out)
	}

	// 5. Remove bot
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"bot", "remove", "default_bot"}, "")
	if code != 0 || !strings.Contains(out, "Removed bot @default_bot") {
		t.Fatalf("bot remove: %d %q", code, out)
	}
}

func TestCLIAliasManagementAndTypeColumn(t *testing.T) {
	env := setupCLI(t)
	ctx := context.Background()

	// 1. Empty alias list
	out, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list"}, "")
	if code != 0 || !strings.Contains(out, "No aliases registered.") {
		t.Fatalf("empty list: %d %q", code, out)
	}

	// 2. Add group alias with thread-id
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"alias", "add", "ops", "-1001234567890", "--thread-id", "42"}, "")
	if code != 0 || !strings.Contains(out, "Added alias ops") || !strings.Contains(out, "Type     group") || !strings.Contains(out, "Thread   42") {
		t.Fatalf("add group alias: %d %q", code, out)
	}

	// 3. Add private alias
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"alias", "add", "alice", "12345"}, "")
	if code != 0 || !strings.Contains(out, "Added alias alice") || !strings.Contains(out, "Type     private") {
		t.Fatalf("add private alias: %d %q", code, out)
	}

	// 4. List aliases - verify TYPE column
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"alias", "list"}, "")
	if code != 0 {
		t.Fatalf("alias list: %d %q", code, out)
	}
	// Check headers
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "TYPE") || !strings.Contains(out, "CHAT ID") || !strings.Contains(out, "THREAD") {
		t.Fatalf("expected TYPE column in headers: %q", out)
	}
	// Check rows
	if !strings.Contains(out, "ops") || !strings.Contains(out, "group") || !strings.Contains(out, "-1001234567890") || !strings.Contains(out, "42") {
		t.Fatalf("expected ops group row: %q", out)
	}
	if !strings.Contains(out, "alice") || !strings.Contains(out, "private") || !strings.Contains(out, "12345") {
		t.Fatalf("expected alice private row: %q", out)
	}

	// 5. JSON output
	jsonOut, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "--json"}, "")
	if code != 0 {
		t.Fatalf("alias list json: %d %q", code, jsonOut)
	}
	var aliases []store.Alias
	if err := json.Unmarshal([]byte(jsonOut), &aliases); err != nil || len(aliases) != 2 {
		t.Fatalf("parse aliases json: %v %q", err, jsonOut)
	}
	if aliases[0].Type == "" || aliases[1].Type == "" {
		t.Fatalf("expected type field in json: %+v", aliases)
	}

	// 5b. Filtered list: group and private
	groupOut, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "group"}, "")
	if code != 0 || !strings.Contains(groupOut, "ops") || strings.Contains(groupOut, "alice") {
		t.Fatalf("alias list group: %d %q", code, groupOut)
	}
	privOut, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "private"}, "")
	if code != 0 || !strings.Contains(privOut, "alice") || strings.Contains(privOut, "ops") {
		t.Fatalf("alias list private: %d %q", code, privOut)
	}
	flagGroupOut, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "--group"}, "")
	if code != 0 || !strings.Contains(flagGroupOut, "ops") || strings.Contains(flagGroupOut, "alice") {
		t.Fatalf("alias list --group: %d %q", code, flagGroupOut)
	}
	flagPrivOut, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "--private"}, "")
	if code != 0 || !strings.Contains(flagPrivOut, "alice") || strings.Contains(flagPrivOut, "ops") {
		t.Fatalf("alias list --private: %d %q", code, flagPrivOut)
	}
	groupJSON, _, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "group", "--json"}, "")
	if code != 0 {
		t.Fatalf("alias list group json: %d %q", code, groupJSON)
	}
	var groupAliases []store.Alias
	if err := json.Unmarshal([]byte(groupJSON), &groupAliases); err != nil || len(groupAliases) != 1 || groupAliases[0].Name != "ops" {
		t.Fatalf("parse group aliases json: %v %q", err, groupJSON)
	}
	_, errOut, code := run(ctx, env.tg, env.wrapper, []string{"alias", "list", "invalid"}, "")
	if code != 1 || !strings.Contains(errOut, "Filter must be group or private") {
		t.Fatalf("alias list invalid filter: %d %q", code, errOut)
	}

	// 6. Update alias
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"alias", "update", "ops", "--name", "incidents", "--thread-id", "99"}, "")
	if code != 0 || !strings.Contains(out, "Updated alias incidents") || !strings.Contains(out, "Thread   99") {
		t.Fatalf("update alias: %d %q", code, out)
	}

	// 7. Remove alias
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"alias", "remove", "incidents"}, "")
	if code != 0 || !strings.Contains(out, "Removed alias incidents") {
		t.Fatalf("remove alias: %d %q", code, out)
	}
}

func TestCLIMessageSending(t *testing.T) {
	env := setupCLI(t)
	ctx := context.Background()

	// Register bot and alias
	run(ctx, env.tg, env.wrapper, []string{"bot", "add"}, "bot_token\n")
	run(ctx, env.tg, env.wrapper, []string{"alias", "add", "ops", "-100123"}, "")

	// 1. Send text to alias
	out, _, code := run(ctx, env.tg, env.wrapper, []string{"msg", "text", "ops", "Deployment complete"}, "")
	if code != 0 || !strings.Contains(out, "Message sent") || !strings.Contains(out, "Chat ID") && strings.Contains(out, "-100123") {
		t.Fatalf("msg to alias: %d %q", code, out)
	}

	// 2. Send text directly to chat ID
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "-100999999", "Direct message to chat ID"}, "")
	if code != 0 || !strings.Contains(out, "Message sent") || !strings.Contains(out, "Chat ID") && strings.Contains(out, "-100999999") {
		t.Fatalf("msg to direct chat ID: %d %q", code, out)
	}

	// 3. Flags: --silent and --parse-mode
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "ops", "*Bold*", "--parse-mode=md", "--silent"}, "")
	if code != 0 || !strings.Contains(out, "Message sent") {
		t.Fatalf("msg with flags: %d %q", code, out)
	}

	// 4. Send photo with local path and caption
	picPath := filepath.Join(t.TempDir(), "test.png")
	os.WriteFile(picPath, []byte("fake-png"), 0o644)
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "photo", "ops", picPath, "--caption", "Daily chart"}, "")
	if code != 0 || !strings.Contains(out, "Message sent") {
		t.Fatalf("msg photo: %d %q", code, out)
	}

	// 5. JSON output
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "ops", "JSON test", "--json"}, "")
	if code != 0 || !strings.Contains(out, `"chatId": "-100123"`) {
		t.Fatalf("msg json: %d %q", code, out)
	}

	// 6. Inline #<thread_id> syntax
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "ops#42", "Thread syntax test"}, "")
	if code != 0 || !strings.Contains(out, "Thread") || !strings.Contains(out, "42") {
		t.Fatalf("msg inline thread: %d %q", code, out)
	}

	// 7. Explicit --thread-id overrides inline #<thread_id>
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "ops#42", "Override test", "--thread-id", "99"}, "")
	if code != 0 || !strings.Contains(out, "Thread") || !strings.Contains(out, "99") {
		t.Fatalf("msg inline thread override: %d %q", code, out)
	}

	// 8. Direct private chat ID ignores thread
	out, _, code = run(ctx, env.tg, env.wrapper, []string{"msg", "text", "123456#42", "Private chat test"}, "")
	if code != 0 || strings.Contains(out, "Thread") {
		t.Fatalf("msg private inline thread ignored: %d %q", code, out)
	}
}

func freePort(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitServerReady(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, err := http.Get(url)
		if err == nil {
			res.Body.Close()
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestCLIServerOptions(t *testing.T) {
	env := setupCLI(t)

	// 1. Both REST and MCP servers disabled -> validation error
	_, errOut, code := run(context.Background(), env.tg, env.wrapper, []string{"serve", "--no-mcp", "--no-rest"}, "")
	if code != 1 || !strings.Contains(errOut, "Cannot disable both REST and MCP servers") {
		t.Fatalf("expected both disabled error: %d %q", code, errOut)
	}

	// 2. Both servers enabled by default: missing MCP config fails fast
	t.Setenv("TELESEND_API_KEY", "test-api-key")
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "")
	_, errOut, code = run(context.Background(), env.tg, env.wrapper, []string{"serve"}, "")
	if code != 1 || !strings.Contains(errOut, "TELESEND_MCP_PUBLIC_URL is required") {
		t.Fatalf("expected missing MCP config error: %d %q", code, errOut)
	}

	// 3. Both servers enabled by default: missing REST config fails fast
	t.Setenv("TELESEND_API_KEY", "")
	t.Setenv("TELESEND_API_KEY_HASH", "")
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "https://mcp.example.com")
	t.Setenv("TELESEND_MCP_OWNER_PASSWORD", "owner_secret")
	_, errOut, code = run(context.Background(), env.tg, env.wrapper, []string{"serve"}, "")
	if code != 1 || !strings.Contains(errOut, "TELESEND_API_KEY or TELESEND_API_KEY_HASH is required") {
		t.Fatalf("expected missing REST config error: %d %q", code, errOut)
	}

	// 4. Start both servers on unified port
	t.Setenv("TELESEND_API_KEY", "test-api-key")
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "https://mcp.example.com")
	t.Setenv("TELESEND_MCP_OWNER_PASSWORD", "owner_secret")
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runResult struct {
		out  string
		err  string
		code int
	}
	resCh := make(chan runResult, 1)
	go func() {
		out, errS, c := run(ctx, env.tg, env.wrapper, []string{"serve", "--port", port}, "")
		resCh <- runResult{out, errS, c}
	}()

	if !waitServerReady("http://127.0.0.1:"+port+"/.well-known/oauth-protected-resource", 2*time.Second) {
		t.Fatalf("server did not become ready on port %s", port)
	}

	// Test MCP discovery endpoint
	res, err := http.Get("http://127.0.0.1:" + port + "/.well-known/oauth-protected-resource")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("expected MCP discovery 200: %v, code %d", err, res.StatusCode)
	}
	res.Body.Close()

	// Test REST health endpoint with API key
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/health", nil)
	req.Header.Set("x-api-key", "test-api-key")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("expected REST health 200: %v, code %d", err, res.StatusCode)
	}
	res.Body.Close()

	// Test REST health endpoint without API key (should be 401)
	res, err = http.Get("http://127.0.0.1:" + port + "/health")
	if err != nil || res.StatusCode != 401 {
		t.Fatalf("expected REST health 401: %v, code %d", err, res.StatusCode)
	}
	res.Body.Close()

	cancel()
	rr := <-resCh
	if !strings.Contains(rr.out, "Telesend server listening on http://127.0.0.1:"+port+"/") ||
		!strings.Contains(rr.out, "REST API:   http://127.0.0.1:"+port+"/") ||
		!strings.Contains(rr.out, "Remote MCP: http://127.0.0.1:"+port+"/") {
		t.Fatalf("expected both server startup output: %q", rr.out)
	}

	// 5. REST only (--no-mcp): MCP config not required
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "")
	t.Setenv("TELESEND_MCP_OWNER_PASSWORD", "")
	portREST := freePort(t)
	ctxREST, cancelREST := context.WithCancel(context.Background())
	defer cancelREST()

	resChREST := make(chan runResult, 1)
	go func() {
		out, errS, c := run(ctxREST, env.tg, env.wrapper, []string{"serve", "--port", portREST, "--no-mcp"}, "")
		resChREST <- runResult{out, errS, c}
	}()

	if !waitServerReady("http://127.0.0.1:"+portREST+"/health", 2*time.Second) {
		t.Fatalf("REST server did not become ready on port %s", portREST)
	}

	// MCP route should return 404
	res, err = http.Get("http://127.0.0.1:" + portREST + "/.well-known/oauth-protected-resource")
	if err != nil || res.StatusCode != 404 {
		t.Fatalf("expected 404 for MCP route on REST-only server: %v, code %d", err, res.StatusCode)
	}
	res.Body.Close()

	cancelREST()
	rrREST := <-resChREST
	if !strings.Contains(rrREST.out, "Telesend REST API listening on http://127.0.0.1:"+portREST+"/") {
		t.Fatalf("expected REST-only startup output: %q", rrREST.out)
	}

	// 6. MCP only (--no-rest): REST config not required
	t.Setenv("TELESEND_API_KEY", "")
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "https://mcp.example.com")
	t.Setenv("TELESEND_MCP_OWNER_PASSWORD", "owner_secret")
	portMCP := freePort(t)
	ctxMCP, cancelMCP := context.WithCancel(context.Background())
	defer cancelMCP()

	resChMCP := make(chan runResult, 1)
	go func() {
		out, errS, c := run(ctxMCP, env.tg, env.wrapper, []string{"serve", "--port", portMCP, "--no-rest"}, "")
		resChMCP <- runResult{out, errS, c}
	}()

	if !waitServerReady("http://127.0.0.1:"+portMCP+"/.well-known/oauth-protected-resource", 2*time.Second) {
		t.Fatalf("MCP server did not become ready on port %s", portMCP)
	}

	// REST route should return 404
	req, _ = http.NewRequest(http.MethodGet, "http://127.0.0.1:"+portMCP+"/health", nil)
	req.Header.Set("x-api-key", "test-api-key")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 404 {
		t.Fatalf("expected 404 for REST route on MCP-only server: %v, code %d", err, res.StatusCode)
	}
	res.Body.Close()

	cancelMCP()
	rrMCP := <-resChMCP
	if !strings.Contains(rrMCP.out, "Telesend remote MCP listening on http://127.0.0.1:"+portMCP+"/") {
		t.Fatalf("expected MCP-only startup output: %q", rrMCP.out)
	}

	// 7. MCP only with --no-api alias
	portAPI := freePort(t)
	ctxAPI, cancelAPI := context.WithCancel(context.Background())
	defer cancelAPI()

	resChAPI := make(chan runResult, 1)
	go func() {
		out, errS, c := run(ctxAPI, env.tg, env.wrapper, []string{"serve", "--port", portAPI, "--no-api"}, "")
		resChAPI <- runResult{out, errS, c}
	}()

	if !waitServerReady("http://127.0.0.1:"+portAPI+"/.well-known/oauth-protected-resource", 2*time.Second) {
		t.Fatalf("MCP server did not become ready on port %s with --no-api", portAPI)
	}
	cancelAPI()
	rrAPI := <-resChAPI
	if !strings.Contains(rrAPI.out, "Telesend remote MCP listening on http://127.0.0.1:"+portAPI+"/") {
		t.Fatalf("expected MCP-only startup output with --no-api: %q", rrAPI.out)
	}

	// 8. Default port is 3100 when --port is omitted
	ctxDef, cancelDef := context.WithCancel(context.Background())
	defer cancelDef()
	resChDef := make(chan runResult, 1)
	go func() {
		out, errS, c := run(ctxDef, env.tg, env.wrapper, []string{"serve", "--no-rest"}, "")
		resChDef <- runResult{out, errS, c}
	}()
	if !waitServerReady("http://127.0.0.1:3100/.well-known/oauth-protected-resource", 2*time.Second) {
		t.Fatalf("server on default port 3100 did not become ready")
	}
	cancelDef()
	rrDef := <-resChDef
	if !strings.Contains(rrDef.out, "http://127.0.0.1:3100/") {
		t.Fatalf("expected default port 3100 in output: %q", rrDef.out)
	}
}
