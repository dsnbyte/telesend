# Telesend

Telesend is a self-hosted Bun application for sending Telegram bot messages from a CLI, local or remote MCP, and an authenticated REST API. Bots, aliases, OAuth grants, and defaults are stored in Bun's built-in SQLite database. There is no administration web UI.

## Install

Requires Bun 1.3 or newer.

```sh
bun install --frozen-lockfile
bun run build
install -m 755 dist/telesend ~/.local/bin/telesend
```

Run from source during development with `bun run src/index.ts`, or use `bun link` to expose the package command.

## Data and configuration

On Linux, the defaults are:

- Database: `${XDG_DATA_HOME:-~/.local/share}/telesend/telesend.db`
- MCP config: `${XDG_CONFIG_HOME:-~/.config}/telesend/config.toml`

macOS uses `~/Library/Application Support/telesend` and `~/Library/Preferences/telesend`. Windows uses `LOCALAPPDATA` and `APPDATA`. Telesend creates the data directory owner-only (`0700`) and the database files owner-only (`0600`) where Unix permissions apply.

## Bots and aliases

Add a bot by entering its BotFather token at the hidden prompt. When stdin is not a terminal, the token is read from stdin. The first bot becomes the default automatically; its display name and username are fetched from Telegram.

```sh
telesend bot add
printf '%s\n' "$TELEGRAM_BOT_TOKEN" | telesend bot add
telesend bot list
telesend bot default alerts_bot
telesend bot remove old_bot
```

Aliases map a short name to a chat ID and, optionally, a Telegram forum topic:

```sh
telesend alias add ops -1001234567890 --thread-id 42
telesend alias list
telesend alias update ops --name incidents --chat-id -1001234567890 --thread-id 99
telesend alias remove incidents
```

An explicit `--thread-id` on a message overrides the thread stored by the alias.

## Send from the CLI

Use `telesend msg` (or the alternative `telesend message`):

```sh
telesend msg text ops "Deployment complete"
telesend msg text ops "Using another bot" --bot alerts_bot --thread-id 42
telesend msg text ops "*Deployment complete*" --parse-mode=md --silent
telesend msg text ops "<b>Deployment complete</b>" --parse-mode=html
telesend msg photo ops ./chart.png --caption "Daily chart"
telesend msg photo ops https://example.com/chart.png
telesend msg document ops file_id:BAACAgQAAxkBAAIB
telesend msg location ops --data '{"latitude":-6.2,"longitude":106.8}'
telesend msg media-group ops --data @album.json
telesend message text ops "Alternative command syntax"
```

A media positional value is interpreted as a local path, except values beginning with `http://`, `https://`, or `file_id:`. Complex operations use `--data` with an inline JSON object or `@file`.

Use `--parse-mode=<html|markdown|md>` with `text` messages to set Telegram's `parse_mode`; `markdown` and `md` use `MarkdownV2`. Use `--silent` with any message type to disable the notification. These flags override the corresponding fields in `--data`.

Supported message types:

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

Other supported Telegram fields can be supplied in `payload`; Telesend does not provide an unrestricted Telegram method proxy.

## REST API

Set a non-empty API key before starting the internet-facing server. Startup fails before binding if it is missing.

```sh
TELESEND_API_KEY='replace-with-a-long-random-secret' telesend serve --host 127.0.0.1 --port 3000
```

Every request, including health checks, requires `x-api-key`. Routes are deliberately unversioned:

- `GET /health`
- `GET|POST /bots`
- `POST /bots/:username/default`
- `DELETE /bots/:username`
- `GET|POST /aliases`
- `PATCH|DELETE /aliases/:name`
- `POST /messages/:type`

```sh
curl -H "x-api-key: $TELESEND_API_KEY" http://127.0.0.1:3000/health

curl -X POST http://127.0.0.1:3000/messages/text \
  -H "x-api-key: $TELESEND_API_KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"ops","messageThreadId":42,"payload":{"text":"Hello"}}'
```

JSON media uses `file_id` or a public URL; server filesystem paths are always rejected:

```json
{
  "to": "ops",
  "bot": "alerts_bot",
  "payload": {
    "photo": { "source": "url", "value": "https://example.com/chart.png" },
    "caption": "Daily chart"
  }
}
```

For a new upload, either provide a file field whose name matches the media payload field, or reference an arbitrary multipart field from `payload`:

```sh
curl -X POST http://127.0.0.1:3000/messages/photo \
  -H "x-api-key: $TELESEND_API_KEY" \
  -F 'to=ops' \
  -F 'payload={"photo":{"source":"upload","value":"image"},"caption":"Daily chart"}' \
  -F 'image=@./chart.png'
```

Requests are limited to 52 MiB. Put an HTTPS reverse proxy or tunnel in front of Telesend; do not expose its plain HTTP listener directly.

## MCP

Telesend serves MCP over stdio. It exposes `list_aliases`, `list_bots`, and one `send_<type>` tool per message type (`send_text`, `send_photo`, `send_media_group`, …). It does not expose bot/alias administration or a generic Telegram method tool.

Example MCP client configuration:

```json
{
  "mcpServers": {
    "telesend": {
      "command": "/home/me/.local/bin/telesend",
      "args": ["mcp", "--config", "/home/me/.config/telesend/config.toml", "--allow-path", "/home/me/reports"]
    }
  }
}
```

Each tool accepts `to`, optional `bot`, optional `messageThreadId`, and `payload`. MCP media sources are `file_id`, `url`, or `path`; raw/base64 file payloads are not accepted.

```json
{
  "to": "ops",
  "messageThreadId": 42,
  "payload": {
    "document": { "source": "path", "value": "/home/me/reports/status.pdf" }
  }
}
```

The working directory is allowed by default. Additional roots come from repeatable `--allow-path` options and TOML. For an allowed root, policy precedence is:

1. Protected secrets, private keys, repository metadata, and active Telesend files are always denied.
2. User `exclude` patterns deny matching files.
3. Built-in source/config/dependency/asset exclusions deny by default.
4. User `include` patterns may override only built-in default exclusions.

Paths are canonicalized before containment checks, so traversal and symlink escapes fail closed. See [the policy reference](docs/mcp-file-policy.md) and [the example configuration](examples/config.toml).

## Remote MCP

Remote MCP is a separate service from the local stdio command and REST API:

- `telesend mcp` communicates over stdio and may use policy-approved local paths.
- `telesend mcp-serve` exposes Streamable HTTP at `/mcp`, requires OAuth 2.1, and accepts only `url` or `file_id` media sources.
- `telesend serve` remains the `x-api-key` REST service. Its `TELESEND_API_KEY` is never used by an MCP connector.

Remote MCP cannot receive a ChatGPT or Claude.ai chat attachment, including a generated image, as file bytes or a local path. To send one, first make it available through a public HTTPS URL that Telegram can fetch, then use `source: "url"`; alternatively, reuse a Telegram `file_id`. Remote MCP does not accept raw bytes, base64/data URLs, or a generic file-upload endpoint.

```json
{
  "to": "ops",
  "payload": {
    "photo": {
      "source": "url",
      "value": "https://files.example.com/daily-chart.png"
    }
  }
}
```

Generate a password hash in a terminal, then put the public HTTPS origin and generated value in `.env` in the directory where Telesend runs:

```sh
bun run mcp:hash-password
```

The command prompts twice without echoing the password. It prints one `TELESEND_MCP_OWNER_PASSWORD_HASH=...` line; copy that complete line into `.env`:

```sh
TELESEND_MCP_PUBLIC_URL=https://telesend.example.com
TELESEND_MCP_OWNER_PASSWORD_HASH=$argon2id$...
```

Bun automatically loads `.env`; shell or process-supervisor variables take precedence. Start the listener with:

```sh
telesend mcp-serve
```

It listens on `http://127.0.0.1:3100` by default. Use `--host` or `--port` only when needed. Keep `.env` out of version control and owner-readable only (`chmod 600 .env`). `TELESEND_MCP_PUBLIC_URL` must be an HTTPS origin without a path, query, or embedded credentials. The connector URL is `https://telesend.example.com/mcp`. The same public origin must also route the OAuth endpoints `/authorize`, `/token`, `/register`, `/revoke`, and `/.well-known/*` to this listener.

The reverse proxy must:

- Terminate HTTPS and proxy every remote MCP/OAuth path to the same single Telesend process.
- Preserve `Authorization`, `Content-Type`, `Accept`, `MCP-Protocol-Version`, and `MCP-Session-Id` headers.
- Disable request-body, authorization-header, and query-string logging for these routes because they may contain passwords, tokens, or short-lived authorization codes.
- Keep the plain HTTP listener private. Multi-process or load-balanced remote MCP serving is not supported yet.

Telesend dynamically registers public OAuth clients, requires PKCE S256, and displays an owner-password consent page. Access is granted with `mcp:read` and/or `mcp:send`; discovery-only grants cannot send messages. Disconnect the connector in ChatGPT or Claude.ai to remove its saved credentials. OAuth clients can also revoke a token at `POST /revoke`; revoking either token invalidates the entire associated grant.

To connect ChatGPT, enable developer mode for a workspace that supports custom MCP apps, create a custom app, and enter `https://telesend.example.com/mcp`. Complete the Telesend consent screen when ChatGPT follows OAuth discovery. Delivery tools are advertised as write actions and may require user confirmation.

To connect Claude.ai, open **Settings → Connectors**, add a custom connector with `https://telesend.example.com/mcp`, and complete the same consent screen. No client secret or Telesend REST API key is required because Telesend supports dynamic client registration.

Do not put an access token, password, authorization code, or `TELESEND_API_KEY` in the connector URL.

## Operations and backup

- Run the REST service as a dedicated unprivileged user and keep its data/config directories owner-only.
- Run remote MCP as the same dedicated user so it opens the same owner-only database, and keep its owner-password hash in an owner-only `.env`, process supervisor, or secret manager.
- Inject `TELESEND_API_KEY` from the process supervisor or secret manager; do not put it in command arguments.
- Terminate HTTPS at a trusted reverse proxy and bind Telesend to a private or loopback address.
- Back up the database with SQLite's online backup command while the service may be running: `sqlite3 telesend.db ".backup telesend-backup.db"`. Copying a live WAL database as one file may produce an inconsistent backup.
- Restore with all Telesend processes stopped, then reapply owner-only permissions.

## Development

```sh
bun run typecheck
bun test
bun run lint
bun run build
```
