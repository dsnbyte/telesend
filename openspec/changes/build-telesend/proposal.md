## Why

Sending Telegram bot messages from automation currently requires each caller to manage bot tokens, Telegram request formats, chat identifiers, and media uploads independently. Telesend will provide one self-hosted Bun application with consistent CLI, MCP, and REST interfaces, backed by local SQLite configuration and supporting multiple bots and reusable recipient aliases.

## What Changes

- Create a Bun-based `telesend` executable with no runtime database dependency beyond Bun's built-in SQLite support.
- Add multi-bot registration that validates tokens with Telegram, stores Telegram-provided identity data, automatically selects the first bot as default, and permits explicit bot selection.
- Add recipient aliases for chat IDs and optional Telegram message thread IDs.
- Support text and every Telegram Bot API message content type exposed by Telesend, including local uploads, URLs, and Telegram file IDs where the content type allows them.
- Provide commands to send messages and manage bots and aliases without a UI.
- Provide an internet-facing REST server protected by the `x-api-key` header and refusing to start without `TELESEND_API_KEY`.
- Provide a stdio MCP server for message delivery, including configurable filesystem allow paths and deny rules for local media.
- Add TOML configuration for MCP filesystem policy while retaining safe built-in exclusions for secrets and application files.

## Capabilities

### New Capabilities

- `bot-management`: Register, inspect, select, and remove multiple Telegram bots, including automatic identity discovery and default-bot behavior.
- `recipient-aliases`: Manage reusable aliases that resolve to Telegram chat IDs and optional message thread IDs.
- `message-delivery`: Validate and deliver Telegram message types through a common service using explicit or default bot and recipient resolution.
- `cli-interface`: Send messages and administer Telesend through the command line.
- `rest-api`: Send messages and administer Telesend through an authenticated internet-facing HTTP API.
- `mcp-interface`: Send messages through a stdio MCP server with controlled access to local files.

### Modified Capabilities

None.

## Impact

- Introduces the complete initial application structure, executable commands, REST and MCP transports, Telegram client, SQLite schema and migrations, TOML configuration, validation, and automated tests.
- Persists Telegram bot tokens and recipient data locally; responses and logs must not disclose bot tokens or API keys.
- Integrates with the Telegram Bot API and inherits its content-specific constraints and delivery failures.
- Adds no web UI and no inbound Telegram update, webhook, or conversation-processing capability.
