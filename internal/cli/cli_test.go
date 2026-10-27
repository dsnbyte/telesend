package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsnbyte/telesend/internal/cli"
	"github.com/dsnbyte/telesend/internal/secret"
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
}

func TestCLIServerOptions(t *testing.T) {
	env := setupCLI(t)

	// Test MCPServe with host and port support
	t.Setenv("TELESEND_MCP_PUBLIC_URL", "https://mcp.example.com")
	hash, _ := secret.Hash("owner_secret")
	t.Setenv("TELESEND_MCP_OWNER_PASSWORD_HASH", secret.Encode(hash))

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	out, _, _ := run(ctx, env.tg, env.wrapper, []string{"mcp-serve", "--host", "0.0.0.0", "--port", "3199"}, "")
	if !strings.Contains(out, "Telesend remote MCP listening on http://0.0.0.0:3199/") {
		t.Fatalf("expected mcp-serve on 0.0.0.0:3199: %q", out)
	}
}
