# Telesend - Agent Guidelines

Send Telegram bot messages via CLI, stdio MCP server, and authenticated REST API. Self-hosted Go application.

## Commands

Always use Go. Run `make check` (or `go test ./... && go vet ./...`) after every modification.

- `make check` — full verification (tests + vet)
- `go test ./internal/<pkg>` — single package test
- `make build` — build standalone binary in `dist/telesend`

## Security Rules

These are hard constraints — never weaken them without explicit approval.

- **No secret leakage.** Bot tokens and API keys must never appear in stdout, stderr, logs, or error responses. Use the existing redaction/error utilities.
- **MCP stdout is sacred.** In MCP mode, stdout is exclusively for JSON-RPC. All other output goes to stderr.
- **MCP file access is fail-closed.** All local file operations in MCP tools must pass authorization checks before proceeding.
- **REST has no filesystem access.** Reject local paths in REST JSON payloads. File uploads use multipart only.
- **REST requires authentication.** The `x-api-key` header is mandatory; fail fast at startup if the key env var is unset.
- **Private file permissions.** Directories `0700`, DB files `0600`.

## Conventions

- **Pure Go SQLite** (`modernc.org/sqlite`). No CGO required for builds or cross-compilation.
- **Database migrations are versioned.** Never modify existing migrations; append new ones.
- **Tests use in-memory databases.** Always `:memory:` path and clean up resources in teardown.

## Workflow

- Touch only what the task requires — no unrelated refactors or style edits.
- Match existing patterns and style.
- Ask when uncertain; flag simpler alternatives.
