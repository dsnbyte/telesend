# Telesend - Agent Guidelines

Self-hosted Bun application for sending Telegram bot messages via CLI, stdio MCP server, and authenticated REST API.

## Development Commands

Always use Bun (do not use npm/yarn/pnpm/node):

- Full verification: `bun run check` (runs typecheck, test, and lint)
- Typecheck: `bun run typecheck` (`tsc --noEmit`)
- Run tests: `bun test` (or `bun test tests/<file>.test.ts`)
- Lint & format check: `bun run lint` (`biome check .`)
- Auto-format: `bun run format` (`biome format --write .`)
- Build binary: `bun run build`

Always verify changes with `bun run check` before concluding a task.

## Architecture & Security Invariants

- **Secrets & Token Redaction**:
  - Never expose Telegram bot tokens or `TELESEND_API_KEY` in stdout, stderr, logs, or error responses.
  - Always wrap/sanitize error messages using `redactSecrets()` or `toAppError()`.
  - Enforce owner-only permissions (`0700` for directories, `0600` for DB files) via `ensurePrivateDirectory`.
- **MCP Stdio Protocol**:
  - Never print arbitrary text to `stdout` during MCP mode. Standard output is reserved strictly for newline-delimited JSON-RPC messages. Use `stderr` or structured loggers if logging is necessary.
  - All local file access in MCP tools must pass `authorizeMcpPayload` / `FilePolicy` checks (fail-closed, canonical paths).
- **REST API Boundaries**:
  - Always require `x-api-key` header; fail fast at startup if `TELESEND_API_KEY` is missing.
  - Reject local filesystem paths in REST JSON payloads. Server filesystem access is forbidden via REST; file uploads must use multipart form data.
- **Database & State**:
  - Use `bun:sqlite` with WAL mode and `PRAGMA foreign_keys = ON`.
  - Manage schema changes strictly via versioned `MIGRATIONS` in `src/db/database.ts`.
  - In unit tests, always use in-memory databases (`databasePath: ":memory:"`) and ensure they are closed (`db.close(false)`) in teardowns.
- **Dependencies & APIs**:
  - Keep dependencies minimal (only `@modelcontextprotocol/server` in production).
  - Use native Bun APIs (`bun:sqlite`, `Bun.argv`, `Response`, `fetch`) instead of adding external libraries.

## Workflow Behavior

### Think Before Coding
- State assumptions explicitly; ask if uncertain.
- Present multiple interpretations instead of silently picking one.
- Flag simpler alternatives and push back when warranted.
- Stop and name the confusion if something is unclear.

### Simplicity First
- Write the minimum code that solves the problem — no speculative features, abstractions, configurability, or error handling for impossible cases.
- Rewrite shorter wherever possible.
- Test: would a senior engineer call this overcomplicated?

### Surgical Changes
- Touch only what the task requires — no unrelated refactors, style edits, or "improvements."
- Match existing style even when you'd do it differently.
- Test: every changed line must trace to the request.
- Run `bun run check` after every modification.
