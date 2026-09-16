## 1. Project Foundation

- [x] 1.1 Initialize the Bun TypeScript package, executable entry point, scripts, strict compiler settings, and source/test layout; verify dependency installation and type checking succeed.
- [x] 1.2 Add the pinned official TypeScript MCP SDK as the only protocol dependency and verify the lockfile contains no database, HTTP, CLI, TOML, glob, or Telegram client package.
- [x] 1.3 Implement platform-aware Telesend data and config path resolution with restrictive directory/file permissions; verify tests cover XDG paths, home fallback, and permission setup.
- [x] 1.4 Define typed application errors, normalized safe result shapes, and secret redaction; verify tests redact API keys, bot tokens, and token-bearing Telegram URLs.

## 2. SQLite Persistence

- [x] 2.1 Implement SQLite startup, migration tracking, WAL, foreign keys, busy timeout, and the initial bots and aliases schema; verify a fresh temporary database reaches the expected schema and constraints.
- [x] 2.2 Implement the bot repository with unique Telegram identity, sole-default enforcement, atomic first-bot/default transitions, and safe projections; verify repository tests cover concurrent first registration, duplicate identities, and default invariants.
- [x] 2.3 Implement the alias repository with CRUD operations and optional positive message thread IDs; verify tests cover uniqueness, updates, deletion, and invalid identifiers.

## 3. Telegram and Core Services

- [x] 3.1 Implement the fetch-based Telegram client, JSON and multipart request construction, upstream response parsing, and safe failures; verify mocked tests cover success, Telegram errors, uploads, and credential redaction.
- [x] 3.2 Implement bot registration through `getMe`, Telegram-derived display name and username storage, listing, default selection, and safe removal; verify all bot-management spec scenarios pass.
- [x] 3.3 Implement recipient parsing and alias resolution with explicit thread override behavior; verify all recipient-alias spec scenarios pass.
- [x] 3.4 Audit the targeted official Telegram Bot API and create the explicit typed message-content catalog with required fields, supported options, and permitted source kinds; verify a catalog test and generated documentation fixture enumerate every in-scope sending content type and no generic method.
- [x] 3.5 Implement shared message validation, bot selection, recipient resolution, Telegram payload mapping, and normalized delivery results for non-media content; verify table-driven tests cover every non-media catalog entry.
- [x] 3.6 Implement `file_id`, URL, local-file, uploaded-file, thumbnail, and multi-item media mapping for every media catalog entry; verify table-driven multipart and source-compatibility tests cover every media operation.

## 4. CLI Interface

- [x] 4.1 Implement the CLI command parser, help/version output, exit-code policy, and stdout/stderr separation; verify CLI snapshot tests cover top-level and invalid-command behavior.
- [x] 4.2 Implement bot add/list/default/remove commands with hidden prompt or stdin token input and no token-valued argument; verify CLI integration tests cover first-default behavior and token-free output/history-safe invocation.
- [x] 4.3 Implement alias add/list/update/remove commands including `--thread-id`; verify CLI integration tests cover the complete alias lifecycle.
- [x] 4.4 Implement content-specific `send` commands with recipient, `--bot`, `--thread-id`, and source options; verify parity tests invoke every catalog entry and local-file upload through the CLI adapter.
- [x] 4.5 Wire `serve` and `mcp` startup commands and service-specific options; verify dispatch tests reach the correct mode without contaminating MCP standard output.

## 5. REST API

- [x] 5.1 Implement REST startup validation that rejects missing or blank `TELESEND_API_KEY` before listening and reports the actual bound address after success; verify process-level tests cover both cases.
- [x] 5.2 Implement constant-time `x-api-key` authentication ahead of routing for every endpoint, including health; verify unauthorized requests cause no repository or Telegram side effects.
- [x] 5.3 Implement unversioned health, bot-management, and alias-management routes with safe JSON responses; verify HTTP contract tests cover CRUD, default selection, `/health`, and rejection of `/v1` paths.
- [x] 5.4 Implement one unversioned `/messages/:type` route per catalog entry with JSON and multipart decoding, request limits, and a hard rejection of server-path sources; verify route parity and multipart tests cover every catalog operation.
- [x] 5.5 Implement stable HTTP status and structured error mapping for authentication, validation, not-found, conflict, and Telegram failures; verify response tests contain actionable details and no secrets.

## 6. MCP Filesystem Policy

- [x] 6.1 Implement TOML loading from the XDG/default path and `--config`, schema validation, home expansion, and fail-closed startup behavior; verify tests cover missing default config, valid config, explicit missing config, invalid TOML, and invalid fields.
- [x] 6.2 Define and document protected and default exclusion pattern sets for secrets, Telesend files, repository metadata, source, config, dependencies, and assets; verify representative files are classified into the intended tier.
- [x] 6.3 Implement allowed-root assembly from current working directory, TOML, and repeatable `--allow-path`, plus canonical containment and regular-file/readability checks; verify traversal, prefix-confusion, broken link, and symlink-escape tests fail closed.
- [x] 6.4 Implement normalized glob precedence for protected patterns, user excludes, default excludes, and user includes; verify tests prove protected and user excludes cannot be overridden while includes can narrowly override defaults.

## 7. MCP Interface

- [x] 7.1 Implement the stdio MCP server lifecycle and content-specific tool schemas without administration or generic Telegram tools; verify protocol tests list exactly the intended tools and keep stdout framing clean.
- [x] 7.2 Connect MCP text and non-file tools to the shared delivery service; verify MCP integration tests cover default and explicit bot selection, aliases, threads, successes, and safe errors.
- [x] 7.3 Connect MCP media tools to URL, `file_id`, and policy-approved local paths without base64 request payloads; verify parity tests cover every media catalog entry and demonstrate denied files are never opened or uploaded.

## 8. Packaging and End-to-End Verification

- [x] 8.1 Add user documentation for installation, data/config locations, bot and alias workflows, the supported message catalog, CLI examples, REST routes/authentication, multipart uploads, MCP setup, and file-policy precedence; verify every documented command and route matches the implemented help/catalog.
- [x] 8.2 Add an example TOML policy and deployment guidance for owner-only data permissions, HTTPS termination, API-key injection, database backup, and MCP allowed roots; verify the example parses with the production config loader.
- [x] 8.3 Add end-to-end tests using a temporary database and mocked Telegram server across CLI, REST, and MCP, including multi-bot selection, threaded aliases, representative text/media delivery, and secret redaction; verify the full Bun test suite passes without network credentials.
- [x] 8.4 Build the distributable Telesend executable and run type checking, tests, formatting/lint checks, and a startup smoke test for CLI, REST, and MCP modes; verify all release checks pass from a clean checkout.
