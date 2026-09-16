## 1. Remote MCP foundations

- [x] 1.1 Inspect the pinned MCP SDK for Streamable HTTP support, make the smallest required version update if needed, and verify `bun install --frozen-lockfile` plus `bun run typecheck` succeed.
- [x] 1.2 Extract transport-neutral MCP tool registration with explicit local and remote media policies, and verify existing stdio MCP tests still pass unchanged.
- [x] 1.3 Add remote tool annotations for read-only discovery and state-changing delivery, and verify the remote tool catalog exposes the expected annotations.
- [x] 1.4 Add a remote payload authorizer that excludes and rejects every nested `path` source while preserving supported URL and `file_id` sources, and verify focused unit tests cover allowed and denied media.

## 2. OAuth persistence and authorization

- [x] 2.1 Add a versioned SQLite migration and repository operations for OAuth clients, grants, authorization codes, access tokens, refresh tokens, expiry, revocation, and token/code digests; verify migration and repository tests use `:memory:` databases and close them.
- [x] 2.2 Add validated remote OAuth configuration for an HTTPS public origin and a separate owner credential, with redacted configuration errors; verify missing, invalid, and valid configuration tests.
- [x] 2.3 Implement OAuth protected-resource and authorization-server metadata plus dynamic client registration with exact redirect-URI persistence, and verify protocol tests cover metadata and rejected malformed registration.
- [x] 2.4 Implement owner-authenticated authorization-code consent with PKCE S256 and scoped grants, and verify invalid owner credentials, invalid PKCE, unapproved scope, and successful redirect cases.
- [x] 2.5 Implement bearer access-token verification, refresh-token rotation, grant/token revocation, expiry checks, and scope enforcement; verify each invalid or revoked credential fails without invoking Telegram.

## 3. Remote HTTP transport

- [x] 3.1 Implement the dedicated loopback-default `telesend mcp-serve` command and remote listener lifecycle without changing `telesend mcp` or `telesend serve`; verify CLI dispatch, help output, and existing command tests.
- [x] 3.2 Implement the remote HTTP handler for OAuth routes and the Streamable HTTP `/mcp` endpoint, including safe unsupported-route/method and request-size failures; verify integration tests initialize and discover tools through the HTTP transport.
- [x] 3.3 Connect authorized remote sessions to the remote-safe tool factory and shared delivery service, and verify discovery-only tokens cannot send while delivery-scoped tokens receive normalized results.
- [x] 3.4 Apply secret redaction and safe response handling to all remote startup, OAuth, and MCP failures, and verify tests assert owner credentials, codes, and tokens never appear in outputs.

## 4. Documentation and end-to-end verification

- [x] 4.1 Document environment configuration, TLS reverse-proxy deployment, session-header forwarding, OAuth redirect/client setup, grant revocation, and ChatGPT/Claude.ai registration without placing secrets in URLs; verify all documented command names and URLs match the CLI.
- [x] 4.2 Add an end-to-end remote MCP authorization and delivery test using a temporary database and mocked Telegram fetch, including rejected server paths and revoked grants; verify `bun test` passes.
- [x] 4.3 Run `bun run check`, inspect the final diff for unintended local stdio or REST changes, and manually validate the documented ChatGPT and Claude.ai connector setup against a public HTTPS deployment.
