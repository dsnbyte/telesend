# Telesend

Send Telegram bot messages from your CLI, AI assistants, or any HTTP client. Self-hosted, single binary, no external dependencies.

Telesend gives you three ways to send messages:

- **CLI** — one-liner from your terminal or shell script
- **MCP** — plug into ChatGPT, Claude.ai, Cursor, VS Code, or any MCP-compatible AI tool
- **REST API** — integrate with your own apps, CI/CD pipelines, or webhooks

## Why Telesend?

- **Let your AI send Telegram messages.** Connect ChatGPT, Codex, Claude, Cursor, Windsurf, VS Code (via Copilot/Continue), or any MCP client — then just ask it to send a message.
- **CI/CD notifications.** Drop the binary into GitHub Actions, GitLab CI, or Jenkins to notify your team on every build.
- **Custom alerting.** Build alerting pipelines via the REST API — monitoring, cron jobs, webhook relays, whatever you need.
- **Quick shell notifications.** `telesend msg text ops "done"` and you're done.

### Example AI Prompts

Once connected via MCP, you can talk to your AI naturally:

> "Send a message to ops saying deployment is complete."

> "Send a photo from https://example.com/chart.png to the monitoring group with caption 'Daily report'."

> "List all my Telegram aliases."

> "Send a markdown message to incidents saying the database is back up."

> "Send the file /home/me/reports/status.pdf to the ops channel."

## Install

One-liner install (downloads the latest binary):

```sh
curl -fsSL https://raw.githubusercontent.com/dsnbyte/telesend/main/install.sh | sh
```

Pick a specific version or install directory:

```sh
curl -fsSL https://raw.githubusercontent.com/dsnbyte/telesend/main/install.sh | VERSION=v0.2.0 sh
curl -fsSL https://raw.githubusercontent.com/dsnbyte/telesend/main/install.sh | INSTALL_DIR=/usr/local/bin sh
```

<details>
<summary><strong>Build from source</strong></summary>

Requires Bun 1.3+.

```sh
bun install --frozen-lockfile
bun run build
install -m 755 dist/telesend ~/.local/bin/telesend
```

During development, run `bun run src/index.ts` or use `bun link`.
</details>

## Quick Start

**1. Add a bot** — paste your BotFather token at the prompt. The first bot becomes the default.

```sh
telesend bot add
telesend bot list
```

**2. Create an alias** — map a short name to a chat ID.

```sh
telesend alias add ops -1001234567890
```

**3. Send a message.**

```sh
telesend msg text ops "Hello from Telesend!"
```

That's it. Read on for the full feature set.

## CLI

Use `telesend msg` (or `telesend message`) to send any supported message type:

```sh
# Simple text
telesend msg text ops "Deployment complete"

# Markdown formatting (uses MarkdownV2)
telesend msg text ops "*Deployment complete*" --parse-mode=md --silent

# HTML formatting
telesend msg text ops "<b>Deployment complete</b>" --parse-mode=html

# Send to a specific bot and forum thread
telesend msg text ops "Alert!" --bot alerts_bot --thread-id 42

# Photos — local file, URL, or Telegram file_id
telesend msg photo ops ./chart.png --caption "Daily chart"
telesend msg photo ops https://example.com/chart.png

# Documents
telesend msg document ops file_id:BAACAgQAAxkBAAIB

# Location and complex payloads
telesend msg location ops --data '{"latitude":-6.2,"longitude":106.8}'
telesend msg media-group ops --data @album.json
```

Media values are treated as local paths unless they start with `http://`, `https://`, or `file_id:`. Use `--data` with inline JSON or `@file` for complex payloads.

### Managing Bots

```sh
telesend bot add                              # interactive token prompt
printf '%s\n' "$TELEGRAM_BOT_TOKEN" | telesend bot add   # non-interactive
telesend bot list
telesend bot default alerts_bot
telesend bot remove old_bot
```

### Managing Aliases

Aliases map a short name to a chat ID and optionally a forum topic:

```sh
telesend alias add ops -1001234567890 --thread-id 42
telesend alias list
telesend alias update ops --name incidents --chat-id -1001234567890 --thread-id 99
telesend alias remove incidents
```

An explicit `--thread-id` on a message overrides the thread stored by the alias.

### Supported Message Types

| Type | Telegram method | Required payload fields |
| --- | --- | --- |
| `text` | `sendMessage` | `text` |
| `message-draft` | `sendMessageDraft` | `draft_id` |
| `rich-message` | `sendRichMessage` | `rich_message` |
| `rich-message-draft` | `sendRichMessageDraft` | `draft_id`, `rich_message` |
| `animation` | `sendAnimation` | `animation` |
| `audio` | `sendAudio` | `audio` |
| `document` | `sendDocument` | `document` |
| `live-photo` | `sendLivePhoto` | `live_photo`, `photo` |
| `photo` | `sendPhoto` | `photo` |
| `sticker` | `sendSticker` | `sticker` |
| `video` | `sendVideo` | `video` |
| `video-note` | `sendVideoNote` | `video_note` |
| `voice` | `sendVoice` | `voice` |
| `paid-media` | `sendPaidMedia` | `star_count`, `media` |
| `media-group` | `sendMediaGroup` | `media` |
| `location` | `sendLocation` | `latitude`, `longitude` |
| `venue` | `sendVenue` | `latitude`, `longitude`, `title`, `address` |
| `contact` | `sendContact` | `phone_number`, `first_name` |
| `poll` | `sendPoll` | `question`, `options` |
| `checklist` | `sendChecklist` | `business_connection_id`, `checklist` |
| `dice` | `sendDice` | none |
| `invoice` | `sendInvoice` | `title`, `description`, `payload`, `currency`, `prices` |
| `game` | `sendGame` | `game_short_name` |

## Local MCP (Cursor, VS Code, Claude Desktop, etc.)

Telesend serves MCP over stdio. Available tools: `list_aliases`, `list_bots`, and one `send_<type>` tool per message type (`send_text`, `send_photo`, `send_media_group`, …).

Add this to your MCP client config:

```json
{
  "mcpServers": {
    "telesend": {
      "command": "/home/me/.local/bin/telesend",
      "args": ["mcp", "--allow-path", "/home/me/reports"]
    }
  }
}
```

Each tool accepts `to`, optional `bot`, optional `messageThreadId`, and `payload`. Media can be `file_id`, `url`, or `path`:

```json
{
  "to": "ops",
  "payload": {
    "document": { "source": "path", "value": "/home/me/reports/status.pdf" }
  }
}
```

The working directory is allowed by default. Use `--allow-path` to grant access to additional directories. Sensitive files (secrets, private keys, etc.) are always denied. See [the policy reference](docs/mcp-file-policy.md) and [the example config](examples/config.toml).

## Remote MCP (ChatGPT, Claude.ai)

For cloud-based AI tools, Telesend exposes Streamable HTTP MCP at `/mcp` with OAuth 2.1 authentication. Remote MCP only accepts `url` or `file_id` media sources — no local file uploads.

### Setup

1. Generate a password hash:

```sh
bun run mcp:hash-password
```

2. Add to `.env` where Telesend runs:

```sh
TELESEND_MCP_PUBLIC_URL=https://telesend.example.com
TELESEND_MCP_OWNER_PASSWORD_HASH=JGFyZ29uMmlkJHY9MTkkLi4u
```

3. Start the listener:

```sh
telesend mcp-serve
```

It listens on `http://127.0.0.1:3100` by default. Put an HTTPS reverse proxy in front of it.

### Connecting ChatGPT

Enable developer mode in a workspace that supports custom MCP apps, create a custom app, and enter `https://telesend.example.com/mcp`. Complete the consent screen when prompted.

### Connecting Claude.ai

Go to **Settings → Connectors**, add a custom connector with `https://telesend.example.com/mcp`, and complete the consent screen. No client secret or API key needed — Telesend handles dynamic client registration.

> **Important:** Never put tokens, passwords, or API keys in the connector URL.

<details>
<summary><strong>Advanced: reverse proxy and OAuth details</strong></summary>

`TELESEND_MCP_PUBLIC_URL` must be an HTTPS origin (no path or credentials). The same origin must route `/mcp`, `/authorize`, `/token`, `/register`, `/revoke`, `/.well-known/*`, and `/icon.png` to the Telesend process.

Your reverse proxy must preserve `Authorization`, `Content-Type`, `Accept`, `MCP-Protocol-Version`, and `MCP-Session-Id` headers. Disable logging of request bodies and auth headers on these routes.

For quick local testing, you can use `TELESEND_MCP_OWNER_PASSWORD` instead of the hash — Telesend will hash it at startup. Prefer the precomputed hash for production.

Access is granted with `mcp:read` and/or `mcp:send` scopes. Disconnect the connector in ChatGPT or Claude.ai to revoke credentials.
</details>

## REST API

For integrations that don't use MCP — webhooks, CI/CD, custom apps.

### Setup

Generate an API key hash:

```sh
bun run rest:hash-api-key
```

Start the server:

```sh
TELESEND_API_KEY='your-secret-key' telesend serve --host 127.0.0.1 --port 3000
```

For production, use `TELESEND_API_KEY_HASH` instead of the plaintext key.

### Endpoints

Every request requires `x-api-key`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/health` | Health check |
| `GET\|POST` | `/bots` | List or add bots |
| `POST` | `/bots/:username/default` | Set default bot |
| `DELETE` | `/bots/:username` | Remove a bot |
| `GET\|POST` | `/aliases` | List or add aliases |
| `PATCH\|DELETE` | `/aliases/:name` | Update or remove alias |
| `POST` | `/messages/:type` | Send a message |

### Examples

```sh
# Health check
curl -H "x-api-key: $TELESEND_API_KEY" http://127.0.0.1:3000/health

# Send text
curl -X POST http://127.0.0.1:3000/messages/text \
  -H "x-api-key: $TELESEND_API_KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"ops","payload":{"text":"Hello"}}'

# Send photo via URL
curl -X POST http://127.0.0.1:3000/messages/photo \
  -H "x-api-key: $TELESEND_API_KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"ops","payload":{"photo":{"source":"url","value":"https://example.com/chart.png"},"caption":"Daily chart"}}'

# Upload a file
curl -X POST http://127.0.0.1:3000/messages/photo \
  -H "x-api-key: $TELESEND_API_KEY" \
  -F 'to=ops' \
  -F 'payload={"photo":{"source":"upload","value":"image"},"caption":"Daily chart"}' \
  -F 'image=@./chart.png'
```

> **Note:** Always put an HTTPS reverse proxy in front of Telesend in production. Requests are limited to 52 MiB. Server filesystem paths are rejected in JSON payloads — use `file_id`, `url`, or multipart upload.

## Data Storage

| | Path |
| --- | --- |
| **Linux** | `~/.local/share/telesend/telesend.db` |
| **macOS** | `~/Library/Application Support/telesend/` |
| **MCP config** | `~/.config/telesend/config.toml` (Linux) |

Back up a running database safely:

```sh
sqlite3 telesend.db ".backup telesend-backup.db"
```

## Development

```sh
bun run typecheck
bun test
bun run lint
bun run build
```
