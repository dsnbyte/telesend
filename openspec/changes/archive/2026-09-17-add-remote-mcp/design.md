## Context

The current application constructs one `McpServer` for `StdioServerTransport` and starts a separately authenticated REST server. MCP payload authorization permits a configured local file path, which is correct only because the stdio client runs on the same machine. The database currently has one versioned migration and application errors are redacted before being returned. See `proposal.md` and `specs/remote-mcp/spec.md` for the requested behavior.

## Goals / Non-Goals

**Goals:**

- Keep local stdio MCP and REST startup behavior backward-compatible.
- Provide a single-owner, self-hosted OAuth 2.1 authorization flow usable by ChatGPT and Claude.ai without handing either connector the REST API key.
- Reuse the existing tool catalog and delivery service while enforcing a stricter remote media policy.

**Non-Goals:**

- Adding multi-user Telesend accounts, SSO, arbitrary identity-provider integration, or an administrative web UI.
- Serving TLS directly from Bun, hosting a connector directory listing, or horizontally scaling MCP sessions in this change.
- Making server-local files available to remote clients.

## Decisions

### Add a dedicated `telesend mcp-serve` process

`telesend mcp-serve [--host <host>] [--port <port>]` will bind a remote-MCP-only HTTP server. It will default to loopback, require an HTTPS `TELESEND_MCP_PUBLIC_URL` for public metadata and redirect validation, and be placed behind an operator-managed TLS reverse proxy or tunnel.

This avoids changing the existing `serve` requirement for `TELESEND_API_KEY` and avoids coupling the remote OAuth surface to the REST administration and delivery routes. `telesend mcp` remains stdio-only.

Alternative considered: add `/mcp` to `telesend serve`. This is smaller at first, but it makes an existing REST command acquire OAuth configuration, combines two independently authenticated public surfaces, and makes least-privilege deployment harder.

### Share tool registration but inject a transport-specific media policy

Extract the common tool catalog registration from the stdio startup path. The local factory will retain `FilePolicy` and the existing `path`, URL, and `file_id` schema. The remote factory will use a no-filesystem policy that omits `path` from input schemas and rejects nested path sources defensively before delivery. Both factories call the same application delivery service and preserve the normalized MCP result/error shape.

Discovery tools will be annotated read-only; every `send_*` tool will be annotated as a state-changing action so hosted clients can display their approval controls.

Alternative considered: proxy remote MCP calls into the stdio process. This adds a local process protocol and leaks the local file capability across a network boundary; separate transports over shared registration are simpler and safer.

### Implement Telesend as a constrained OAuth 2.1 authorization server

Remote startup will require a dedicated owner credential such as `TELESEND_MCP_OWNER_PASSWORD`, distinct from `TELESEND_API_KEY`. The owner authenticates at the authorization endpoint and explicitly approves a connector's requested scopes. Telesend will expose protected-resource and authorization-server metadata, require authorization-code flow with PKCE S256, support refresh-token rotation and revocation, and accept dynamic client registration for hosted clients that do not provide a pre-registered client.

The grant model is deliberately single-owner: all authorized connectors act with the Telesend instance owner's registered bots and aliases. Tokens are scoped to `mcp:read` and `mcp:send`; the server checks scope before tool execution.

Authorization codes, access tokens, refresh tokens, and the owner password will never be stored or logged in plaintext. Database state will store only token/code digests plus expiry, grant, client, redirect URI, and scope metadata. A versioned SQLite migration will add the client, grant, authorization-code, access-token, and refresh-token tables with foreign keys and expiry indexes. Owner-password verification will use a strong password-derived digest held in configuration; an implementation must select a Bun/native primitive already available before adding a dependency.

Alternative considered: reuse `TELESEND_API_KEY` as a bearer token or require it as a header. It does not offer user consent, scope separation, or revocation and is unsuitable for first-class hosted connector setup. Alternative considered: delegate authentication to a third-party OIDC provider. It would reduce local OAuth code but adds a mandatory external service and provider-specific setup to a self-hosted, single-owner tool.

### Route OAuth and MCP endpoints through one remote server handler

The remote process will serve the Streamable HTTP MCP endpoint and the required OAuth discovery, authorization, registration, token, and revocation endpoints from the configured public origin. It will reject unexpected methods, malformed JSON-RPC, oversized requests, cross-origin browser requests not required by the flow, and unauthenticated MCP requests before creating a session or calling Telegram.

The implementation will use the current MCP SDK HTTP transport if it supports the required Streamable HTTP behavior; otherwise it will make the smallest compatible SDK upgrade and lock the version. MCP transport sessions remain process-local in this release, so a reverse proxy must preserve the required MCP session headers and route a session to the same process.

### Preserve safe operational boundaries

The remote command will emit only a safe listener/public-endpoint startup message. All HTTP, OAuth, and MCP errors will flow through existing redaction helpers with remote secrets supplied as redaction values. The documentation will state that reverse proxies must terminate TLS, forward the public origin correctly, and avoid logging authorization headers, form bodies, or query strings carrying authorization codes.

## Risks / Trade-offs

- [OAuth implementation mistakes could expose or accept credentials] → Require PKCE, exact redirect-URI matching, one-time short-lived authorization codes, hashed persisted artifacts, token rotation, redacted errors, and focused negative tests.
- [Dynamic client registration increases client-trust surface] → Require an interactive owner approval for every grant and persist the exact registered redirect URI and client identity before issuing a code.
- [Hosted client transport behavior differs from the installed SDK] → Validate against an MCP protocol client plus manual ChatGPT and Claude.ai connector setup; upgrade only the MCP SDK when required.
- [Public endpoint is accidentally exposed without TLS] → Require an HTTPS public URL while binding loopback by default and make reverse-proxy/TLS setup explicit in documentation.
- [Long-lived Streamable HTTP sessions do not scale across processes] → Declare a single-process deployment constraint and preserve session headers; defer shared session storage/load balancing.
- [Remote delivery is a write action vulnerable to unintended prompts] → Mark delivery tools as mutating, grant `mcp:send` explicitly, and keep all existing delivery validation.

## Migration Plan

1. Add the database migration and remote OAuth configuration validation without changing existing commands.
2. Add `mcp-serve`, metadata/OAuth routes, and the remote MCP transport behind its required configuration.
3. Deploy it on loopback behind a TLS reverse proxy using the configured public HTTPS URL; register the redirect URI and complete connector authorization in ChatGPT and Claude.ai.
4. Verify discovery, scoped delivery, token refresh/revocation, rejected path media, and unchanged local stdio/REST behavior before relying on it.
5. Roll back by stopping `mcp-serve` or removing its reverse-proxy route; local MCP and REST continue unchanged. Revoke persisted grants before a later re-enable if access must be reset.

## Open Questions

- The exact environment variable name and password-digest input format can be selected during implementation, provided it remains a separate owner credential and no plaintext credential is persisted or logged.
