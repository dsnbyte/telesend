## Why

Telesend's MCP server only speaks local stdio, so ChatGPT and Claude.ai cannot call its Telegram delivery tools. A standards-compatible remote MCP endpoint allows those hosted clients to use the same delivery capability without exposing the server's static REST API key.

## What Changes

- Add an authenticated, HTTPS-ready Streamable HTTP MCP endpoint for remote clients while retaining `telesend mcp` as the unchanged local stdio mode.
- Add OAuth 2.1 authorization for the remote MCP surface, including scoped, revocable user grants and token refresh, so ChatGPT and Claude.ai can connect without receiving `TELESEND_API_KEY`.
- Expose the existing read-only discovery and delivery tools through the remote endpoint, with remote tool annotations that identify delivery as a write action.
- Reject all server-local `path` media sources over remote MCP; retain only URL and Telegram `file_id` sources remotely. Local stdio MCP keeps its existing file-policy behavior.
- Document public HTTPS/reverse-proxy deployment and how to register the endpoint in ChatGPT and Claude.ai.

## Capabilities

### New Capabilities

- `remote-mcp`: Public Streamable HTTP MCP access, OAuth authorization, and remote-safe delivery behavior for hosted MCP clients.

### Modified Capabilities

- None. The current OpenSpec capability inventory has no active specifications; local stdio MCP behavior remains unchanged.

## Impact

- Affects MCP server construction and transport lifecycle, CLI/server startup, HTTP routing, database migrations/state, error handling, tests, and README deployment guidance.
- Requires the MCP SDK's HTTP transport support and may require updating the pinned `@modelcontextprotocol/server` version if the current release lacks the needed stable transport APIs.
- Adds a publicly reachable HTTPS endpoint intended for ChatGPT custom MCP apps and Claude.ai custom connectors; TLS termination remains the deployment operator's responsibility.
