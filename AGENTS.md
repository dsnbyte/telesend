# Telesend - Agent Guidelines

Send Telegram bot messages via CLI, stdio MCP server, and authenticated REST API. Self-hosted Bun application.

## Commands

Always use Bun. Run `bun run check` after every modification.

- `bun run check` — full verification (typecheck + test + lint)
- `bun test tests/<file>.test.ts` — single test file
- `bun run format` — auto-format

## Security Rules

These are hard constraints — never weaken them without explicit approval.

- **No secret leakage.** Bot tokens and API keys must never appear in stdout, stderr, logs, or error responses. Use the existing redaction/error utilities.
- **MCP stdout is sacred.** In MCP mode, stdout is exclusively for JSON-RPC. All other output goes to stderr.
- **MCP file access is fail-closed.** All local file operations in MCP tools must pass authorization checks before proceeding.
- **REST has no filesystem access.** Reject local paths in REST JSON payloads. File uploads use multipart only.
- **REST requires authentication.** The `x-api-key` header is mandatory; fail fast at startup if the key env var is unset.
- **Private file permissions.** Directories `0700`, DB files `0600`.

## Conventions

- **One production dependency** (`@modelcontextprotocol/server`). Prefer native Bun APIs over new packages.
- **Database migrations are versioned.** Never modify existing migrations; append new ones.
- **Tests use in-memory databases.** Always `:memory:` path and `db.close(false)` in teardown.

## Workflow

- Touch only what the task requires — no unrelated refactors or style edits.
- Match existing patterns and style.
- Ask when uncertain; flag simpler alternatives.
