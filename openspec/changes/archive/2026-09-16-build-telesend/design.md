## Context

Telesend is a greenfield project: the repository currently contains OpenSpec configuration but no application source, package manifest, database, or compatibility constraints. See `proposal.md` for motivation and the delta specs for observable behavior.

The application must expose the same delivery behavior through a short-lived CLI process, a long-running REST process, and a stdio MCP process. Bot tokens are persistent secrets; the REST service is internet-facing; and MCP local-path delivery gives model-driven calls access to the server filesystem. Those constraints require shared validation and explicit security boundaries even though the initial codebase should remain small.

## Goals / Non-Goals

**Goals:**

- Build one Bun executable whose CLI, REST, and MCP adapters invoke the same application services.
- Keep SQLite, HTTP, Telegram requests, TOML parsing, glob matching, and file handling on Bun-native APIs where available.
- Make the supported Telegram message catalog explicit, typed, documented, and testable rather than exposing arbitrary Telegram methods.
- Fail closed for REST authentication and MCP filesystem policy.
- Keep credentials out of normal outputs, structured responses, and logs.

**Non-Goals:**

- Receiving Telegram updates, webhooks, commands, or conversations.
- A browser UI, multi-user accounts, roles, or per-client authorization policies.
- Queueing, scheduling, retry workers, delivery history, or cross-host database synchronization.
- Automatically supporting future Telegram methods without adding an explicit Telesend operation.
- Encrypting bot tokens against a fully compromised host; filesystem permissions and host security define the local trust boundary.

## Decisions

### Use a small shared-core architecture

The executable will be divided into transport adapters, application services, and infrastructure adapters:

```text
CLI --------+
REST -------+--> application services --> Telegram client
MCP --------+            |
                            +-------------> SQLite repositories
```

Bot management, alias resolution, message validation, bot selection, delivery result normalization, and safe error mapping live below the transports. Each transport only parses its input, invokes a service, and renders the result. This avoids three subtly different implementations without introducing a general dependency-injection framework.

Alternative considered: implement each interface independently. It has fewer initial modules but duplicates security and delivery behavior and makes parity tests difficult.

### Use Bun-native infrastructure with one protocol-focused dependency

Use `bun:sqlite`, `Bun.serve`, `fetch`, `Bun.file`, `FormData`, `Bun.TOML.parse`, and `Bun.Glob`. Parse CLI arguments in-project because the command tree is bounded. Use the official TypeScript MCP SDK for protocol framing and schemas rather than hand-implementing MCP JSON-RPC. No database ORM, Telegram SDK, HTTP framework, CLI framework, dotenv package, TOML package, or glob package is needed.

The “zero dependency” SQLite constraint is therefore preserved: the database layer uses Bun's built-in driver. The MCP SDK is retained because protocol compatibility is a different concern and a manual implementation would be disproportionately risky.

### Persist operational data in SQLite

Use one database in the Telesend data directory with migrations applied on process startup.

```text
bots
- id INTEGER PRIMARY KEY
- telegram_id TEXT NOT NULL UNIQUE
- name TEXT NOT NULL
- username TEXT NOT NULL UNIQUE
- token TEXT NOT NULL
- is_default INTEGER NOT NULL DEFAULT 0
- created_at TEXT NOT NULL
- updated_at TEXT NOT NULL

aliases
- id INTEGER PRIMARY KEY
- name TEXT NOT NULL UNIQUE
- chat_id TEXT NOT NULL
- message_thread_id INTEGER NULL
- created_at TEXT NOT NULL
- updated_at TEXT NOT NULL

schema_migrations
- version INTEGER PRIMARY KEY
- applied_at TEXT NOT NULL
```

Telegram and chat IDs are stored as decimal text to preserve identity without numeric coercion at transport boundaries. A partial unique index on `bots(is_default) WHERE is_default = 1` enforces at most one default. Registration performs Telegram `getMe` before opening a write transaction, then uses an immediate transaction to insert the verified identity and assign default status based on whether a bot already exists. The database enables foreign keys, WAL mode, and a busy timeout for concurrent CLI, REST, and MCP processes.

The data directory is created with owner-only permissions where the platform supports them, and the database is owner-readable/writable. Tokens remain stored in SQLite because Telesend must operate unattended; encrypting them with a key available to the same process would not protect against host compromise.

### Derive bot identity from Telegram

`bot add` and `POST /bots` accept only the token as identity input. Telegram `getMe` supplies `telegram_id`, `first_name`, optional `last_name`, and `username`. `name` is the space-joined display name; `username` is the unique operator-facing selector and is stored without requiring an `@` prefix. Failed verification writes nothing. Duplicate Telegram IDs are rejected rather than treated as updates.

Alternative considered: accept a local bot name. This conflicts with the chosen requirement that both name and username reflect Telegram, and introduces another identifier without a demonstrated need.

### Define one explicit message catalog

A central typed catalog will enumerate the content-specific operations supported by the release, their required and optional fields, accepted media sources, Telegram method names, and response shape. CLI commands, REST routes, and MCP tools will be wired from or checked against this catalog, while their transport-specific encodings remain explicit and testable.

The initial implementation will audit the current official Telegram Bot API and include every method whose primary purpose is sending message content. It will include Telegram's conventional text and media types as well as content types such as contacts, locations, venues, polls, dice, games, invoices, media groups, paid media, live photos, and rich messages when available in the targeted Bot API. Copying, forwarding, chat actions, editing, deletion, chat administration, and arbitrary method invocation are not message content types and are outside the initial catalog unless separately specified.

This release-based catalog avoids the misleading promise of automatic future compatibility. Adding a new Telegram content type requires a catalog entry, validation, all three public adapters, documentation, and contract tests.

### Model media by source, not by transport payload

The application service receives a discriminated media source:

```text
file_id | url | local_file | uploaded_file
```

CLI and MCP can create `local_file` after their own path checks. REST can create `uploaded_file` from multipart content but cannot create `local_file`. The Telegram adapter turns uploads into `multipart/form-data` and passes URL or file ID strings directly when allowed for the selected operation. This keeps REST clients from probing server paths and avoids base64 expansion in MCP requests.

### Use TOML for MCP filesystem policy

The default config is `${XDG_CONFIG_HOME:-~/.config}/telesend/config.toml`; `telesend mcp --config` can select another file. Bun parses the file at startup, after which Telesend validates its schema. There is no hot reload.

```toml
[mcp.files]
allow_paths = ["/home/user/reports"]
include = ["reports/**/*.json"]
exclude = ["**/private/**", "**/*.secret.*"]
```

The effective allowed roots are the current working directory plus configured roots plus repeatable CLI `--allow-path` values. Configured roots must be absolute after home expansion; relative CLI roots resolve against the startup working directory. Paths are canonicalized before containment checks.

File policy precedence is:

1. Reject non-regular, unreadable files and files outside allowed roots.
2. Reject protected patterns and resolved Telesend config/database paths; these cannot be overridden.
3. Reject user `exclude` matches.
4. For built-in default exclusions, permit only when a user `include` matches.
5. Permit the file.

Protected exclusions cover environment files, private-key material, repository metadata, and Telesend's own data/configuration. Default exclusions cover source extensions, project configuration, dependency directories, and conventional application directories such as `src`, `config`, and `assets`. Patterns match normalized root-relative paths using Bun's native glob semantics. Missing default config is valid; an explicitly selected missing file or any present invalid file fails startup.

Alternative considered: allow every OS-readable path. That makes an MCP tool an unrestricted exfiltration primitive. Encoding bytes in MCP was also rejected because it wastes context and bypasses server-side path policy.

### Authenticate REST before routing operations

`telesend serve` validates a trimmed, non-empty `TELESEND_API_KEY` before calling the listener API. Every request, including health checks, must supply `x-api-key`. Comparison uses a constant-time byte comparison after checking equal length. Authentication errors are uniform and never log the supplied or expected key.

The initial route shape is unversioned:

```text
GET    /health
GET    /bots
POST   /bots
DELETE /bots/:username
POST   /bots/:username/default
GET    /aliases
POST   /aliases
PATCH  /aliases/:name
DELETE /aliases/:name
POST   /messages/:type
```

JSON is used when a message has no uploaded file; multipart is used for uploads. REST never accepts `source: path`. The server is suitable for internet exposure only behind HTTPS termination; TLS certificate management is outside Telesend.

### Keep errors typed internally and safe externally

Application errors use a small stable set: validation, unauthorized, not found, conflict, filesystem policy, configuration, and Telegram upstream failure. Transport adapters map those errors to exit codes, HTTP statuses, or MCP tool errors. A centralized redactor removes known API keys, bot tokens, and token-bearing Telegram URL segments from all rendered errors and logs.

Telegram success responses are normalized to include the selected bot username, resolved chat ID, message thread when present, and Telegram result. The transports do not return the bot token or internal database row IDs.

### Test contracts at boundaries

Unit tests use temporary SQLite databases and injected fetch behavior for Telegram calls. Contract tests exercise the same application service through CLI, HTTP, and MCP adapters, with focused tests for first-bot races, sole-default enforcement, multipart handling, REST authentication before side effects, path traversal and symlink escapes, glob precedence, secret redaction, and clean MCP stdout framing. A small opt-in integration test may target a real Telegram bot, but the normal test suite must not require network credentials.

## Risks / Trade-offs

- [Telegram adds or changes message types frequently] -> Treat support as a versioned release catalog and require catalog, adapter, documentation, and contract-test updates together.
- [A compromised host can read stored bot tokens] -> Restrict data-file permissions, never expose tokens through interfaces, and document the host as the trust boundary.
- [SQLite has multiple local writers] -> Use short transactions, WAL, a busy timeout, unique constraints, and atomic default selection.
- [MCP glob rules may surprise operators] -> Ship documented defaults, expose policy-specific errors, normalize paths consistently, and test precedence and symlink cases.
- [Default exclusions can block legitimate code or JSON documents] -> Permit narrow user `include` patterns to override only default exclusions, never protected or explicit user exclusions.
- [REST multipart uploads can consume substantial resources] -> Enforce request and content limits before or during ingestion and defer final acceptance to Telegram's content-specific limits.
- [The official MCP SDK adds a runtime dependency] -> Pin it, isolate it behind the MCP adapter, and avoid spreading its types into the application core.

## Migration Plan

This is a greenfield deployment, so there is no legacy data migration.

1. Initialize the data and configuration directories with restrictive permissions.
2. Apply the initial SQLite schema transactionally on first command startup.
3. Register at least one Telegram bot; it becomes the default automatically.
4. Add recipient aliases as needed.
5. Configure MCP filesystem policy and verify denied-path behavior before enabling MCP clients.
6. Set `TELESEND_API_KEY`, place REST behind HTTPS termination, and start the server.

Rollback consists of stopping Telesend and restoring a pre-upgrade copy of the SQLite database if a future schema migration fails. Initial installation can be removed by deleting the executable and, only when intentional, its data and configuration directories.
