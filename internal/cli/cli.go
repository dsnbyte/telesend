// Package cli implements the Telesend command line interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/jsonx"
	"github.com/dsnbyte/telesend/internal/mcp"
	"github.com/dsnbyte/telesend/internal/paths"
	"github.com/dsnbyte/telesend/internal/rest"
	"github.com/dsnbyte/telesend/internal/secret"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/telegram"
	"github.com/dsnbyte/telesend/internal/ui"
	"github.com/dsnbyte/telesend/internal/version"
	"golang.org/x/term"
)

// IO holds stdout and stderr.
type IO struct {
	Out io.Writer
	Err io.Writer
	In  io.Reader
}

// DefaultIO returns standard descriptors.
func DefaultIO() IO { return IO{Out: os.Stdout, Err: os.Stderr, In: os.Stdin} }

// Run executes a command line with the application core.
func Run(ctx context.Context, args []string, a *app.App, ioStreams IO) int {
	jsonMode := false
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" {
			jsonMode = true
		} else {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 || filtered[0] == "help" || filtered[0] == "--help" || filtered[0] == "-h" {
		fmt.Fprintln(ioStreams.Out, Help())
		return 0
	}
	if filtered[0] == "--version" || filtered[0] == "-V" || filtered[0] == "version" {
		fmt.Fprintln(ioStreams.Out, version.Version)
		return 0
	}

	err := dispatch(ctx, filtered, a, ioStreams, jsonMode)
	if err != nil {
		fmt.Fprintf(ioStreams.Err, "%s\n", formatError(err))
		return 1
	}
	return 0
}

func dispatch(ctx context.Context, args []string, a *app.App, ioStreams IO, jsonMode bool) error {
	switch args[0] {
	case "bot":
		return runBot(ctx, args[1:], a, ioStreams, jsonMode)
	case "alias":
		return runAlias(args[1:], a, ioStreams, jsonMode)
	case "msg", "message":
		return runMsg(ctx, args[1:], a, ioStreams, jsonMode)
	case "serve":
		return runServe(ctx, args[1:], a, ioStreams)
	case "mcp":
		return runMCP(ctx, args[1:], a, ioStreams)
	case "hash-key":
		return runHashKey(ioStreams)
	case "hash-password":
		return runHashPassword(ioStreams)
	default:
		return apperr.Newf(apperr.Validation, "Unknown command: %s", args[0])
	}
}

func runBot(ctx context.Context, args []string, a *app.App, ioStreams IO, jsonMode bool) error {
	if len(args) == 0 {
		return apperr.New(apperr.Validation, "Usage: telesend bot <add|list|default|remove>")
	}
	switch args[0] {
	case "add":
		token, err := readPassword(ioStreams, "Bot token: ")
		if err != nil {
			return err
		}
		bot, err := a.RegisterBot(ctx, token)
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(bot)))
		} else {
			fmt.Fprintf(ioStreams.Out, "Added bot @%s\nName         %s\nTelegram ID  %s\nDefault      %s\n",
				bot.Username, bot.Name, bot.TelegramID, yesNo(bot.IsDefault))
		}
		return nil
	case "list":
		bots, err := a.Bots.List()
		if err != nil {
			return err
		}
		if jsonMode {
			if bots == nil {
				bots = []store.Bot{}
			}
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(bots)))
			return nil
		}
		if len(bots) == 0 {
			fmt.Fprintln(ioStreams.Out, "No bots registered.")
			return nil
		}
		rows := make([][]string, len(bots))
		for i, b := range bots {
			rows[i] = []string{b.Username, b.Name, b.TelegramID, yesNo(b.IsDefault)}
		}
		fmt.Fprintln(ioStreams.Out, formatTable([]string{"USERNAME", "NAME", "ID", "DEFAULT"}, rows))
		return nil
	case "default":
		if len(args) < 2 || args[1] == "" {
			return apperr.New(apperr.Validation, "Bot username is required")
		}
		bot, err := a.Bots.SetDefault(args[1])
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(bot)))
		} else {
			fmt.Fprintf(ioStreams.Out, "Default bot set to @%s\n", bot.Username)
		}
		return nil
	case "remove":
		if len(args) < 2 || args[1] == "" {
			return apperr.New(apperr.Validation, "Bot username is required")
		}
		bot, err := a.Bots.Remove(args[1])
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(bot)))
		} else {
			fmt.Fprintf(ioStreams.Out, "Removed bot @%s\nName         %s\nTelegram ID  %s\nDefault      %s\n",
				bot.Username, bot.Name, bot.TelegramID, yesNo(bot.IsDefault))
		}
		return nil
	case "update", "edit":
		if len(args) < 2 || args[1] == "" {
			return apperr.New(apperr.Validation, "Bot username is required")
		}
		opts, _, err := parseOptions(args[2:])
		if err != nil {
			return err
		}
		name := opts["name"]
		if name == "" {
			return apperr.New(apperr.Validation, "--name is required to update bot")
		}
		bot, err := a.UpdateBot(ctx, args[1], store.BotInput{Name: name})
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(bot)))
		} else {
			fmt.Fprintf(ioStreams.Out, "Updated bot @%s\nName         %s\nTelegram ID  %s\nDefault      %s\n",
				bot.Username, bot.Name, bot.TelegramID, yesNo(bot.IsDefault))
		}
		return nil
	default:
		return apperr.New(apperr.Validation, "Usage: telesend bot <add|list|default|remove>")
	}
}

func runAlias(args []string, a *app.App, ioStreams IO, jsonMode bool) error {
	if len(args) == 0 {
		return apperr.New(apperr.Validation, "Usage: telesend alias <add|list|update|remove>")
	}
	switch args[0] {
	case "add":
		if len(args) < 3 {
			return apperr.New(apperr.Validation, "Usage: telesend alias add <name> <chat-id> [--thread-id <id>]")
		}
		name, chatID := args[1], args[2]
		opts, _, err := parseOptions(args[3:])
		if err != nil {
			return err
		}
		var thread *int
		if t, ok := opts["thread-id"]; ok {
			v, err := strconv.Atoi(t)
			if err != nil || v <= 0 {
				return apperr.New(apperr.Validation, "--thread-id must be a positive integer")
			}
			thread = &v
		}
		alias, err := a.Aliases.Create(store.AliasInput{Name: name, ChatID: chatID, MessageThreadID: thread})
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(alias)))
		} else {
			th := "-"
			if alias.MessageThreadID != nil {
				th = strconv.Itoa(*alias.MessageThreadID)
			}
			fmt.Fprintf(ioStreams.Out, "Added alias %s\nType     %s\nChat ID  %s\nThread   %s\n",
				alias.Name, alias.Type, alias.ChatID, th)
		}
		return nil
	case "list":
		var filter string
		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "--group":
				if filter != "" && filter != store.TypeGroup {
					return apperr.New(apperr.Validation, "Conflicting filter arguments")
				}
				filter = store.TypeGroup
			case arg == "--private":
				if filter != "" && filter != store.TypePrivate {
					return apperr.New(apperr.Validation, "Conflicting filter arguments")
				}
				filter = store.TypePrivate
			case arg == "--type" || arg == "--filter":
				if i+1 >= len(args) {
					return apperr.Newf(apperr.Validation, "%s requires a value", arg)
				}
				i++
				if filter != "" && filter != args[i] {
					return apperr.New(apperr.Validation, "Conflicting filter arguments")
				}
				filter = args[i]
			case strings.HasPrefix(arg, "--type="):
				v := strings.TrimPrefix(arg, "--type=")
				if filter != "" && filter != v {
					return apperr.New(apperr.Validation, "Conflicting filter arguments")
				}
				filter = v
			case strings.HasPrefix(arg, "--filter="):
				v := strings.TrimPrefix(arg, "--filter=")
				if filter != "" && filter != v {
					return apperr.New(apperr.Validation, "Conflicting filter arguments")
				}
				filter = v
			case arg == "--json":
				jsonMode = true
			case strings.HasPrefix(arg, "-"):
				return apperr.Newf(apperr.Validation, "Unknown flag: %s", arg)
			default:
				if filter != "" && filter != arg {
					return apperr.Newf(apperr.Validation, "Unexpected argument: %s", arg)
				}
				filter = arg
			}
		}
		aliases, err := a.Aliases.List(filter)
		if err != nil {
			return err
		}
		if jsonMode {
			if aliases == nil {
				aliases = []store.Alias{}
			}
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(aliases)))
			return nil
		}
		if len(aliases) == 0 {
			fmt.Fprintln(ioStreams.Out, "No aliases registered.")
			return nil
		}
		rows := make([][]string, len(aliases))
		for i, al := range aliases {
			th := "-"
			if al.MessageThreadID != nil {
				th = strconv.Itoa(*al.MessageThreadID)
			}
			rows[i] = []string{al.Name, al.Type, al.ChatID, th}
		}
		fmt.Fprintln(ioStreams.Out, formatTable([]string{"NAME", "TYPE", "CHAT ID", "THREAD"}, rows))
		return nil
	case "update":
		if len(args) < 2 {
			return apperr.New(apperr.Validation, "Alias name is required")
		}
		name := args[1]
		existing, err := a.Aliases.Find(name)
		if err != nil {
			return err
		}
		if existing == nil {
			return apperr.Newf(apperr.NotFound, "Alias %q was not found", name)
		}
		opts, _, err := parseOptions(args[2:])
		if err != nil {
			return err
		}
		in := store.AliasInput{
			Name:            existing.Name,
			ChatID:          existing.ChatID,
			MessageThreadID: existing.MessageThreadID,
		}
		if n, ok := opts["name"]; ok {
			in.Name = n
		}
		if c, ok := opts["chat-id"]; ok {
			in.ChatID = c
		}
		if t, ok := opts["thread-id"]; ok {
			v, err := strconv.Atoi(t)
			if err != nil || v <= 0 {
				return apperr.New(apperr.Validation, "--thread-id must be a positive integer")
			}
			in.MessageThreadID = &v
		}
		updated, err := a.Aliases.Update(name, in)
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(updated)))
		} else {
			th := "-"
			if updated.MessageThreadID != nil {
				th = strconv.Itoa(*updated.MessageThreadID)
			}
			fmt.Fprintf(ioStreams.Out, "Updated alias %s\nType     %s\nChat ID  %s\nThread   %s\n",
				updated.Name, updated.Type, updated.ChatID, th)
		}
		return nil
	case "remove":
		if len(args) < 2 {
			return apperr.New(apperr.Validation, "Alias name is required")
		}
		removed, err := a.Aliases.Remove(args[1])
		if err != nil {
			return err
		}
		if jsonMode {
			fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(removed)))
		} else {
			th := "-"
			if removed.MessageThreadID != nil {
				th = strconv.Itoa(*removed.MessageThreadID)
			}
			fmt.Fprintf(ioStreams.Out, "Removed alias %s\nType     %s\nChat ID  %s\nThread   %s\n",
				removed.Name, removed.Type, removed.ChatID, th)
		}
		return nil
	default:
		return apperr.New(apperr.Validation, "Usage: telesend alias <add|list|update|remove>")
	}
}

func runMsg(ctx context.Context, args []string, a *app.App, ioStreams IO, jsonMode bool) error {
	if len(args) < 2 {
		return apperr.New(apperr.Validation, "Usage: telesend msg <type> <alias|chat-id>[#<thread_id>] [content] [options]")
	}
	typ, to := args[0], args[1]
	op, ok := telegram.Get(typ)
	if !ok {
		return apperr.Newf(apperr.Validation, "Unsupported message type: %s", typ)
	}

	opts, positionals, silent, err := parseMsgArgs(args[2:])
	if err != nil {
		return err
	}

	parseMode := opts["parse-mode"]
	if parseMode != "" {
		if typ != "text" {
			return apperr.New(apperr.Validation, "--parse-mode is only supported for text messages")
		}
		switch strings.ToLower(parseMode) {
		case "html":
			parseMode = "HTML"
		case "markdown", "md":
			parseMode = "MarkdownV2"
		default:
			return apperr.New(apperr.Validation, "--parse-mode must be html, markdown, or md")
		}
	}

	var payload map[string]any
	if data := opts["data"]; data != "" {
		raw := data
		if strings.HasPrefix(raw, "@") {
			content, err := os.ReadFile(raw[1:])
			if err != nil {
				return apperr.Newf(apperr.Validation, "Cannot read data file: %v", err)
			}
			raw = string(content)
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
			return apperr.New(apperr.Validation, "--data must contain a JSON object or @file path")
		}
	} else if typ == "text" {
		if len(positionals) == 0 {
			return apperr.New(apperr.Validation, "Message text is required")
		}
		payload = map[string]any{"text": strings.Join(positionals, " ")}
	} else if len(op.MediaFields) == 1 && len(positionals) > 0 {
		payload = map[string]any{op.MediaFields[0]: parseMediaArg(positionals[0])}
	} else if typ == "dice" {
		payload = map[string]any{}
	} else {
		return apperr.Newf(apperr.Validation, "Use --data '<json>' to provide fields for %s", typ)
	}

	if cap := opts["caption"]; cap != "" {
		payload["caption"] = cap
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if silent {
		payload["disable_notification"] = true
	}

	var thread int
	if t, ok := opts["thread-id"]; ok {
		var err error
		thread, err = strconv.Atoi(t)
		if err != nil || thread <= 0 {
			return apperr.New(apperr.Validation, "--thread-id must be a positive integer")
		}
	}

	res, err := a.Send(ctx, app.SendRequest{
		Type:     typ,
		To:       to,
		Bot:      opts["bot"],
		ThreadID: thread,
		Payload:  payload,
	})
	if err != nil {
		return err
	}

	if jsonMode {
		fmt.Fprintln(ioStreams.Out, string(jsonx.MarshalIndent(res)))
		return nil
	}
	fields := [][2]string{
		{"Bot", "@" + res.Bot},
		{"Chat ID", res.ChatID},
	}
	if res.ThreadID != 0 {
		fields = append(fields, [2]string{"Thread", strconv.Itoa(res.ThreadID)})
	}
	if res.MessageID != 0 {
		fields = append(fields, [2]string{"Message ID", strconv.FormatInt(res.MessageID, 10)})
	}
	fmt.Fprintf(ioStreams.Out, "Message sent\n%s\n", formatFields(fields))
	return nil
}

func parseMediaArg(val string) map[string]any {
	if strings.HasPrefix(val, "file_id:") {
		return map[string]any{"source": "file_id", "value": val[len("file_id:"):]}
	}
	if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
		return map[string]any{"source": "url", "value": val}
	}
	return map[string]any{"source": "path", "value": val}
}

func runServe(ctx context.Context, args []string, a *app.App, ioStreams IO) error {
	noMCP := false
	noREST := false
	noUI := false
	var filtered []string
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "--no-mcp":
			noMCP = true
		case "--no-rest", "--no-api":
			noREST = true
		case "--no-ui":
			noUI = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if noMCP && noREST && noUI {
		return apperr.New(apperr.Validation, "Cannot disable all services (REST, MCP, and UI)")
	}

	opts, positionals, err := parseOptions(filtered)
	if err != nil {
		return err
	}
	if len(positionals) > 0 {
		return apperr.Newf(apperr.Validation, "Unexpected argument: %s", positionals[0])
	}
	host := "127.0.0.1"
	if h, ok := opts["host"]; ok && h != "" {
		host = h
	}
	port := 3100
	if p, ok := opts["port"]; ok {
		port, err = strconv.Atoi(p)
		if err != nil || port <= 0 {
			return apperr.New(apperr.Validation, "--port must be a positive integer")
		}
	}

	var restHandler http.Handler
	if !noREST {
		v, err := secret.NewVerifier(os.Getenv("TELESEND_API_KEY_HASH"), os.Getenv("TELESEND_API_KEY"))
		if err != nil {
			return err
		}
		restHandler, err = rest.NewServer(a, v)
		if err != nil {
			return err
		}
	}

	var mcpHandler http.Handler
	var mcpURL *url.URL
	if !noMCP {
		publicRaw := os.Getenv("TELESEND_MCP_PUBLIC_URL")
		if publicRaw == "" {
			return apperr.New(apperr.Configuration, "TELESEND_MCP_PUBLIC_URL is required")
		}
		publicURL, err := url.Parse(publicRaw)
		if err != nil {
			return apperr.New(apperr.Configuration, "TELESEND_MCP_PUBLIC_URL must be an HTTPS origin")
		}
		v, err := secret.NewVerifier(os.Getenv("TELESEND_MCP_PASSWORD_HASH"), os.Getenv("TELESEND_MCP_PASSWORD"))
		if err != nil {
			return err
		}
		mcpHandler, err = mcp.NewRemoteServer(a, mcp.RemoteConfig{
			PublicURL:     publicURL,
			OwnerVerifier: v,
		})
		if err != nil {
			return err
		}
		u := *publicURL
		u.Path = "/mcp"
		mcpURL = &u
	}

	var uiHandler http.Handler
	if !noUI {
		v, err := secret.NewVerifier(os.Getenv("TELESEND_UI_PASSWORD_HASH"), os.Getenv("TELESEND_UI_PASSWORD"))
		if err != nil {
			return err
		}
		uiHandler, err = ui.NewServer(a, v)
		if err != nil {
			return err
		}
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	boundHostPort := net.JoinHostPort(host, portStr)

	servicesCount := 0
	if !noREST {
		servicesCount++
	}
	if !noMCP {
		servicesCount++
	}
	if !noUI {
		servicesCount++
	}

	if servicesCount > 1 {
		fmt.Fprintf(ioStreams.Out, "Telesend server listening on http://%s/\n", boundHostPort)
		if !noREST {
			fmt.Fprintf(ioStreams.Out, "  REST API:   http://%s/\n", boundHostPort)
		}
		if !noMCP {
			fmt.Fprintf(ioStreams.Out, "  Remote MCP: http://%s/ (public %s)\n", boundHostPort, mcpURL.String())
		}
		if !noUI {
			fmt.Fprintf(ioStreams.Out, "  Web UI:     http://%s/\n", boundHostPort)
		}
	} else if !noREST {
		fmt.Fprintf(ioStreams.Out, "Telesend REST API listening on http://%s/\n", boundHostPort)
	} else if !noMCP {
		fmt.Fprintf(ioStreams.Out, "Telesend remote MCP listening on http://%s/ (public %s)\n", boundHostPort, mcpURL.String())
	} else if !noUI {
		fmt.Fprintf(ioStreams.Out, "Telesend Web UI listening on http://%s/\n", boundHostPort)
	}

	server := &http.Server{Handler: combinedServerHandler{rest: restHandler, mcp: mcpHandler, ui: uiHandler}}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	err = server.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type combinedServerHandler struct {
	rest http.Handler
	mcp  http.Handler
	ui   http.Handler
}

func (h combinedServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isMCPPath(r.URL.Path) {
		if h.mcp != nil {
			h.mcp.ServeHTTP(w, r)
			return
		}
		if (r.URL.Path == "/icon.png" || r.URL.Path == "/favicon.ico" || r.URL.Path == "/apple-touch-icon.png" || r.URL.Path == "/telesend-icon.png") && h.ui != nil {
			h.ui.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	if isRESTPath(r.URL.Path) {
		if h.rest != nil {
			h.rest.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	if h.rest != nil && r.Header.Get("x-api-key") != "" {
		h.rest.ServeHTTP(w, r)
		return
	}
	if isUIPath(r.URL.Path) {
		if h.ui != nil {
			h.ui.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	if h.rest != nil && h.mcp == nil && h.ui == nil {
		h.rest.ServeHTTP(w, r)
		return
	}
	if h.mcp != nil && h.rest == nil && h.ui == nil {
		h.mcp.ServeHTTP(w, r)
		return
	}
	if h.ui != nil {
		h.ui.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func isUIPath(p string) bool {
	switch p {
	case "/", "/ui", "/ui/", "/app.css", "/app.js", "/ui/app.css", "/ui/app.js":
		return true
	}
	return strings.HasPrefix(p, "/api/ui/") || strings.HasPrefix(p, "/ui/")
}

func isMCPPath(p string) bool {
	switch p {
	case "/mcp", "/register", "/authorize", "/token", "/revoke",
		"/icon.png", "/favicon.ico", "/apple-touch-icon.png", "/telesend-icon.png":
		return true
	}
	return strings.HasPrefix(p, "/.well-known/") || strings.HasPrefix(p, "/mcp/")
}

func isRESTPath(p string) bool {
	switch p {
	case "/health", "/bots", "/aliases", "/messages":
		return true
	}
	return strings.HasPrefix(p, "/bots/") ||
		strings.HasPrefix(p, "/aliases/") ||
		strings.HasPrefix(p, "/messages/")
}

func runMCP(ctx context.Context, args []string, a *app.App, ioStreams IO) error {
	var allowPaths []string
	var configPath string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--allow-path" && i+1 < len(args) {
			allowPaths = append(allowPaths, args[i+1])
			i++
		} else if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
		} else {
			return apperr.New(apperr.Validation, "MCP options must be --config or --allow-path with a value")
		}
	}

	p, err := paths.Default()
	if err != nil {
		return err
	}
	explicit := configPath != ""
	if configPath == "" {
		configPath = p.ConfigFile
	}
	cfg, err := mcp.LoadConfig(configPath, explicit, os.Getenv("HOME"))
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	policy, err := mcp.NewFilePolicy(mcp.FilePolicyOptions{
		CWD:             cwd,
		ConfiguredRoots: cfg.MCP.Files.AllowPaths,
		CLIRoots:        allowPaths,
		Include:         cfg.MCP.Files.Include,
		Exclude:         cfg.MCP.Files.Exclude,
		ProtectedPaths:  []string{configPath, a.DatabasePath},
	})
	if err != nil {
		return err
	}

	server := mcp.NewStdioServer(a, policy)
	return server.ServeStdio(ctx, ioStreams.In, ioStreams.Out)
}

func runHashKey(ioStreams IO) error {
	key, err := readPassword(ioStreams, "API key: ")
	if err != nil {
		return err
	}
	if key == "" {
		return apperr.New(apperr.Validation, "API key is required")
	}
	confirm, err := readPassword(ioStreams, "Confirm API key: ")
	if err != nil {
		return err
	}
	if key != confirm {
		return apperr.New(apperr.Validation, "API key confirmation does not match")
	}
	hash, err := secret.Hash(key)
	if err != nil {
		return err
	}
	fmt.Fprintf(ioStreams.Out, "TELESEND_API_KEY_HASH=%s\n", secret.Encode(hash))
	return nil
}

func runHashPassword(ioStreams IO) error {
	pw, err := readPassword(ioStreams, "Owner password: ")
	if err != nil {
		return err
	}
	if pw == "" {
		return apperr.New(apperr.Validation, "Owner password is required")
	}
	confirm, err := readPassword(ioStreams, "Confirm owner password: ")
	if err != nil {
		return err
	}
	if pw != confirm {
		return apperr.New(apperr.Validation, "Owner password confirmation does not match")
	}
	hash, err := secret.Hash(pw)
	if err != nil {
		return err
	}
	fmt.Fprintf(ioStreams.Out, "TELESEND_MCP_PASSWORD_HASH=%s\n", secret.Encode(hash))
	return nil
}

func parseOptions(args []string) (opts map[string]string, positionals []string, err error) {
	opts = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			if strings.Contains(name, "=") {
				k, v, _ := strings.Cut(name, "=")
				opts[k] = v
				continue
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, nil, apperr.Newf(apperr.Validation, "%s requires a value", a)
			}
			opts[name] = args[i+1]
			i++
		} else {
			positionals = append(positionals, a)
		}
	}
	return opts, positionals, nil
}

func parseMsgArgs(args []string) (opts map[string]string, positionals []string, silent bool, err error) {
	opts = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--silent" {
			silent = true
			continue
		}
		if strings.HasPrefix(a, "--parse-mode=") {
			v := a[len("--parse-mode="):]
			if v == "" {
				return nil, nil, false, apperr.New(apperr.Validation, "Use --parse-mode=<html|markdown|md>")
			}
			opts["parse-mode"] = v
			continue
		}
		if a == "--parse-mode" {
			return nil, nil, false, apperr.New(apperr.Validation, "Use --parse-mode=<html|markdown|md>")
		}
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			if strings.Contains(name, "=") {
				k, v, _ := strings.Cut(name, "=")
				opts[k] = v
				continue
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, nil, false, apperr.Newf(apperr.Validation, "%s requires a value", a)
			}
			opts[name] = args[i+1]
			i++
		} else {
			positionals = append(positionals, a)
		}
	}
	return opts, positionals, silent, nil
}

func formatFields(fields [][2]string) string {
	max := 0
	for _, f := range fields {
		if len(f[0]) > max {
			max = len(f[0])
		}
	}
	lines := make([]string, len(fields))
	for i, f := range fields {
		lines[i] = fmt.Sprintf("%-*s  %s", max, f[0], f[1])
	}
	return strings.Join(lines, "\n")
}

func formatTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	line := func(cells []string) string {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = fmt.Sprintf("%-*s", widths[i], c)
		}
		return strings.TrimRight(strings.Join(out, "  "), " ")
	}
	dashes := make([]string, len(widths))
	for i, w := range widths {
		dashes[i] = strings.Repeat("-", w)
	}
	lines := []string{line(headers), line(dashes)}
	for _, row := range rows {
		lines = append(lines, line(row))
	}
	return strings.Join(lines, "\n")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatError(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return fmt.Sprintf("Error: %s", apperr.Redact(appErr.Message))
	}
	return fmt.Sprintf("Unexpected error: %s", apperr.Redact(err.Error()))
}

func readPassword(ioStreams IO, prompt string) (string, error) {
	if f, ok := ioStreams.In.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(ioStreams.Err, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(ioStreams.Err)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	// Non-interactive fallback
	var line string
	_, err := fmt.Fscanln(ioStreams.In, &line)
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// Help returns the CLI usage text.
func Help() string {
	types := make([]string, len(telegram.Catalog))
	for i, op := range telegram.Catalog {
		types[i] = op.Type
	}
	return fmt.Sprintf(`Telesend %s

Usage:
  telesend [--json] bot add|list|default|remove|update
  telesend [--json] alias add <name> <chat-id> [--thread-id <id>]
  telesend [--json] alias list [group|private]
  telesend [--json] alias update|remove
  telesend [--json] msg|message <type> <alias|chat-id>[#<thread_id>] [content] [--data <json|@file>] [--bot <username>] [--thread-id <id>] [--silent] [--parse-mode=<html|markdown|md>]
  telesend serve [--host <host>] [--port <port>] [--no-mcp] [--no-rest] [--no-ui]
  telesend mcp [--config <path>] [--allow-path <path> ...]
  telesend hash-key
  telesend hash-password
  telesend --version

Options:
  --json           Print JSON instead of human-readable text
  -V, --version    Print the installed version

Message types:
  %s`, version.Version, strings.Join(types, ", "))
}
