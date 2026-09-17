# remote-mcp Specification

## Purpose

Provide a secure remote MCP interface that lets hosted AI clients send Telegram messages through Telesend without replacing the existing local stdio integration.

## Requirements

### Requirement: Serve a remote Streamable HTTP MCP endpoint
The system SHALL expose a single Streamable HTTP MCP endpoint suitable for a public HTTPS deployment. It SHALL support MCP initialization, tool discovery, and tool invocation for supported remote MCP clients.

#### Scenario: Hosted client connects
- **WHEN** a compatible client connects to the configured public MCP endpoint over HTTPS with valid authorization
- **THEN** the system completes MCP initialization and exposes its remote tool catalog

#### Scenario: Unsupported route or method
- **WHEN** a request does not target the remote MCP endpoint with an MCP-supported method
- **THEN** the system returns a safe HTTP error without performing a delivery action

### Requirement: Preserve local stdio MCP behavior
The system SHALL retain `telesend mcp` as a local stdio MCP server with its existing configuration and file-policy behavior. Remote MCP availability SHALL NOT require changing an existing local client configuration.

#### Scenario: Existing local client starts
- **WHEN** an operator starts `telesend mcp` with an existing valid MCP configuration
- **THEN** it communicates exclusively over stdio and continues to enforce its configured local file policy

### Requirement: Require OAuth authorization for remote MCP
The system SHALL require OAuth 2.1 bearer-token authorization before it initializes a remote MCP session or executes a remote tool. It SHALL advertise authorization metadata compatible with remote MCP client discovery and support authorization-code grants with PKCE and refresh-token renewal.

#### Scenario: Client authorizes successfully
- **WHEN** a user completes the configured authorization flow and a client presents a valid, unexpired access token with the required scope
- **THEN** the system permits the remote MCP request

#### Scenario: Request has no valid access token
- **WHEN** a remote MCP request lacks a valid, unexpired bearer token with the required scope
- **THEN** the system rejects it without exposing credentials, authorization codes, tokens, or delivery details

#### Scenario: User revokes a grant
- **WHEN** an authorized user revokes a remote MCP grant
- **THEN** access and refresh tokens associated with that grant can no longer authorize remote MCP requests

### Requirement: Require a hashed remote-MCP owner password
The system SHALL require `TELESEND_MCP_OWNER_PASSWORD_HASH` to start `telesend mcp-serve`. The value SHALL be a Bun-compatible password hash. It SHALL provide `bun run mcp:hash-password` to interactively generate the hash without echoing the entered password.

#### Scenario: Operator configures a generated password hash
- **WHEN** an operator stores a Bun-compatible password hash in `TELESEND_MCP_OWNER_PASSWORD_HASH`
- **THEN** `telesend mcp-serve` uses it to verify the OAuth owner consent password

#### Scenario: Operator omits the password hash
- **WHEN** `telesend mcp-serve` starts without `TELESEND_MCP_OWNER_PASSWORD_HASH`
- **THEN** it exits before binding a listener with an actionable configuration error

#### Scenario: Operator generates a password hash
- **WHEN** an operator runs `bun run mcp:hash-password` in an interactive terminal
- **THEN** the command confirms the password without echoing it and writes a complete `TELESEND_MCP_OWNER_PASSWORD_HASH` assignment to standard output

### Requirement: Scope remote MCP access
The system SHALL issue remote MCP grants with separate discovery and message-delivery scopes. It SHALL allow discovery tools only with discovery scope and delivery tools only with delivery scope.

#### Scenario: Discovery-only grant lists recipients
- **WHEN** a client with discovery scope invokes a discovery tool
- **THEN** the system returns the tool result without permitting a message delivery action

#### Scenario: Discovery-only grant sends a message
- **WHEN** a client with no delivery scope invokes a delivery tool
- **THEN** the system rejects the invocation without contacting Telegram

### Requirement: Expose remote-safe tools
The system SHALL expose the existing recipient and bot discovery tools and content-specific message-delivery tools through remote MCP. It SHALL identify every delivery tool as an action that can modify external state, while discovery tools remain read-only.

#### Scenario: Client discovers tools
- **WHEN** an authorized remote client requests the tool catalog
- **THEN** the response distinguishes read-only discovery tools from message-delivery actions

#### Scenario: Authorized delivery
- **WHEN** a client with delivery scope invokes a valid message-delivery tool
- **THEN** the system applies the same delivery validation and returns the same normalized result contract as the local MCP interface

### Requirement: Disallow server-local files and binary uploads over remote MCP
The system SHALL reject every remote MCP media source whose type is `path`. Remote MCP SHALL accept only the source types already supported without server filesystem access: Telegram `file_id`, and URL where that message type supports URL media. It SHALL NOT accept raw bytes, base64/data URLs, or an upload reference for a chat attachment or generated file.

#### Scenario: Remote client submits a server path
- **WHEN** a remote MCP tool payload contains a media source with type `path`
- **THEN** the system rejects the request before reading the server filesystem

#### Scenario: Remote client submits an allowed URL source
- **WHEN** a remote MCP tool payload uses a URL source for a message type that supports URL media
- **THEN** the system validates and delivers it without reading a server-local file

#### Scenario: Hosted chat attachment has no public URL
- **WHEN** a hosted MCP client attempts to send a chat attachment or generated file without a public URL or Telegram `file_id`
- **THEN** the system rejects it without storing or uploading file bytes

### Requirement: Provide deployment and connector guidance
The system SHALL document public deployment prerequisites, including HTTPS termination, OAuth client redirect registration, token-secret handling, and the endpoint registration steps for ChatGPT and Claude.ai.

#### Scenario: Operator follows deployment documentation
- **WHEN** an operator follows the documented deployment and connector setup
- **THEN** they can expose the endpoint behind HTTPS without placing OAuth tokens or `TELESEND_API_KEY` in a connector URL or application log
