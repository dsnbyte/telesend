# Telesend

Send Telegram bot messages from your CLI, AI assistants, or any HTTP client. Self-hosted, single binary, no external dependencies.

Telesend gives you multiple ways to send and manage messages:

- **CLI** — one-liner from your terminal or shell script
- **MCP** — plug into ChatGPT, Claude.ai, Cursor, VS Code, or any MCP-compatible AI tool
- **REST API** — integrate with your own apps, CI/CD pipelines, or webhooks
- **Web UI** — clean browser dashboard to manage bots, aliases, test messages, view history, and export database backups

## Why Telesend?

- **Let your AI send Telegram messages.** Connect ChatGPT, Claude, Gemini, Codex, Cursor, Pi Code, or any MCP client — then just ask it to send a message.
- **CI/CD notifications.** Drop the binary into GitHub Actions, GitLab CI, or Jenkins to notify your team on every build.
- **Custom alerting.** Build alerting pipelines via the REST API — monitoring, cron jobs, webhook relays, whatever you need.
- **Quick shell notifications.** `telesend msg text ops "done"` and you're done.

### Example AI Prompts

Once connected via MCP, you can talk to your AI naturally:

> "Send a message to ops saying deployment is complete."

> "Send a photo from https://example.com/chart.png to the monitoring group with caption 'Daily report'."

> "List all my Telegram aliases."

> "Send this update to every group alias."

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

Requires Go 1.22+.

```sh
make build
# Or directly:
go build -ldflags="-s -w" -o dist/telesend ./cmd/telesend
install -m 755 dist/telesend ~/.local/bin/telesend
```

During development, run `go run ./cmd/telesend` or `make check`.
</details>

Confirm the installed version:

```sh
telesend --version
telesend -V
```

### Single Binary Distribution

Telesend is built and distributed as a single standalone binary. Everything—the embedded SQLite database engine, HTTP server, Remote MCP support, and Web UI—is compiled right into one self-contained executable with zero runtime dependencies. No Docker, Python, Node.js, or external database required. Just download the binary for your platform, drop it onto your system, and it is ready to run.

<details>
<summary><strong>Instant Deploy to Nibrun</strong></summary>

Looking for a hassle-free, zero-config cloud deployment? You can deploy Telesend to [nibrun.com](https://nibrun.com) instantly and for free:

1. Create a new app at [nibrun.com](https://nibrun.com).
2. Point it to the Linux binary URL from the [GitHub Releases](https://github.com/dsnbyte/telesend/releases) page (e.g. `https://github.com/dsnbyte/telesend/releases/latest/download/telesend-linux-x64`).
3. Set the following environment variables in your app settings:
   - `TELESEND_API_KEY` — secret key for REST API authentication
   - `TELESEND_DATA_DIR` — **must be set to `"./data"`** (ensures SQLite database and configs persist on nibrun's persistent volume)
   - `TELESEND_MCP_PASSWORD` — password for Remote MCP connections
   - `TELESEND_UI_PASSWORD` — password to access the Web UI
   - `TELESEND_MCP_PUBLIC_URL` — *(optional)* can be configured after deployment finishes once you obtain your public URL from nibrun (e.g. `https://<app>.nibrun.app`)
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

You can send to an alias or directly to a Telegram chat ID:

```sh
# Send to an alias
telesend msg text ops "Hello from Telesend!"

# Or send directly to a chat ID (no alias needed)
telesend msg text -1001234567890 "Hello to group!"
telesend msg text 123456789 "Hello to private chat!"
```

That's it. Read on for the full feature set.

CLI commands print human-readable text. Lists are shown as tables. Pass `--json` for machine-readable JSON. MCP tools and the REST API always return JSON.

```sh
telesend bot list
telesend alias list --json
```

## CLI

Use `telesend msg` (or `telesend message`) to send any supported message type:

```sh
# Send text to an alias
telesend msg text ops "Deployment complete"

# Send text directly to a chat ID (no alias needed)
# Negative IDs are groups/supergroups/channels; positive IDs are private chats:
telesend msg text -1001234567890 "Deployment complete to group"
telesend msg text 123456789 "Direct message to user"

# Markdown formatting (uses MarkdownV2)
telesend msg text ops "*Deployment complete*" --parse-mode=md --silent

# HTML formatting
telesend msg text ops "<b>Deployment complete</b>" --parse-mode=html

# Send to a specific bot and forum thread (via flag or #<thread_id> syntax)
telesend msg text ops "Alert!" --bot alerts_bot --thread-id 42
telesend msg text ops#42 "Alert to forum topic"
telesend msg text -1001234567890#42 "Alert to forum topic directly"

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
telesend bot default alerts_bot	 # set default bot
telesend bot remove old_bot	 # remove a bot
```

### Managing Aliases

Aliases map a short name to a chat ID and optionally a forum topic. Names may include spaces (quote them in the shell). Each alias is classified as `group` (negative chat ID, including supergroups and channels) or `private`:

```sh
telesend alias add ops -1001234567890 --thread-id 42
telesend alias add "Team Report" -1002603419700#23   # direct chat ID with topic thread ID
telesend alias add "Team Chat" -1002603419700
telesend alias add alice 123456789
telesend alias list
telesend alias list group       # list only group aliases
telesend alias list private     # list only private aliases
telesend alias update ops --name incidents --chat-id -1001234567890 --thread-id 99
telesend alias update "Team Report" -1002603419700#50 # update chat ID and topic thread ID directly
telesend alias remove incidents

# View delivery history (table or JSON)
telesend history
telesend history list --limit 20 --type text
telesend history --json
telesend history prune          # manually prune records older than retention period
telesend history clear          # clear all delivery history
```

The `telesend alias list` command prints an alias table with a **TYPE** column classifying each alias as `group` (negative chat IDs, including supergroups and channels) or `private`. It accepts an optional `group` or `private` filter argument to list only matching aliases:

```text
NAME       TYPE     CHAT ID         THREAD
---------  -------  --------------  ------
ops        group    -1001234567890  42
Team Chat  group    -1002603419700  -
alice      private  123456789       -
```

An explicit `--thread-id` or `#<thread_id>` appended to the recipient (e.g. `ops#42` or `-1001234567890#42`) targets a forum thread in a group and overrides the thread stored by an alias. If the destination is a private chat, thread IDs are ignored. MCP `list_aliases` supports filtering by `type` or `filter` (`group` or `private`), and both MCP and `telesend alias list --json` include `type` so automated tools can distinguish group and private recipients.

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

The working directory is allowed by default. Use `--allow-path` to grant access to additional directories. Sensitive files (secrets, private keys, etc.) are always denied. See [the policy reference](docs/mcp-file-policy.md) and [the example config](examples/config.json).

## Remote MCP (ChatGPT, Claude.ai)

For cloud-based AI tools, Telesend exposes Streamable HTTP MCP at `/mcp` with OAuth 2.1 authentication. Remote MCP only accepts `url` or `file_id` media sources — no local file uploads.

### Setup

1. Generate a password hash:

```sh
telesend hash-password
```

2. Add to `.env` where Telesend runs:

```sh
TELESEND_MCP_PUBLIC_URL=https://telesend.example.com
TELESEND_MCP_PASSWORD_HASH=replace-with-generated-hash
```

3. Start the server:

```sh
telesend serve
```

By default it listens on `http://127.0.0.1:3100` and starts the Web UI, REST API, and Remote MCP server. Use `--no-rest`, `--no-mcp`, or `--no-ui` to disable specific services:

```sh
telesend serve --no-rest --no-ui
```

Use `--host` and `--port` to customize the address (for example, to listen on `0.0.0.0` in Docker or a container environment):

```sh
telesend serve --host 0.0.0.0 --port 3100
```

Put an HTTPS reverse proxy in front of it in production.

### Connecting ChatGPT

Enable developer mode in a workspace that supports custom MCP apps, create a custom app, and enter `https://telesend.example.com/mcp`. Complete the consent screen when prompted.

### Connecting Claude.ai

Go to **Settings → Connectors**, add a custom connector with `https://telesend.example.com/mcp`, and complete the consent screen. No client secret or API key needed — Telesend handles dynamic client registration.

### Connecting Amazon Quick (Custom OAuth)

For platforms that require manual OAuth client configuration (such as Amazon Quick):

1. Register an OAuth client with your platform's callback/redirect URL:

```sh
curl -X POST https://telesend.example.com/register \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Amazon Quick",
    "redirect_uris": [
      "https://quick.aws.com/sn/oauthcallback",
      "https://us-east-1.quicksight.aws.amazon.com/sn/oauthcallback"
    ]
  }'
```

Copy the `client_id` from the JSON response.

2. In Amazon Quick, create a connector under **Connectors → Create for your team → Model Context Protocol (MCP)** using endpoint `https://telesend.example.com/mcp`.
3. Under authentication, choose **Custom OAuth app**:
   - **Client ID:** Paste the `client_id` from step 1.
   - **Public OAuth client:** Check this box (Telesend uses public PKCE; no client secret needed).
   - **Authorization URL:** `https://telesend.example.com/authorize`
   - **Token URL:** `https://telesend.example.com/token`
4. Choose **Create and continue** and log in with `TELESEND_MCP_PASSWORD` on the consent screen.

> **Important:** Never put tokens, passwords, or API keys in the connector URL.

<details>
<summary><strong>Advanced: reverse proxy and OAuth details</strong></summary>

`TELESEND_MCP_PUBLIC_URL` must be an HTTPS origin (no path or credentials). The same origin must route `/mcp`, `/authorize`, `/token`, `/register`, `/revoke`, `/.well-known/*`, and `/icon.png` to the Telesend process.

Your reverse proxy must preserve `Authorization`, `Content-Type`, `Accept`, `MCP-Protocol-Version`, and `MCP-Session-Id` headers. Disable logging of request bodies and auth headers on these routes.

For quick local testing, you can use `TELESEND_MCP_PASSWORD` instead of the hash — Telesend will hash it at startup. Prefer the precomputed hash for production.

Access is granted with `mcp:read` and/or `mcp:send` scopes. Disconnect the connector in ChatGPT or Claude.ai to revoke credentials.
</details>

## REST API

For integrations that don't use MCP — webhooks, CI/CD, custom apps.

### Setup

Generate an API key hash:

```sh
telesend hash-key
```

Start the server:

```sh
# Start both REST API and Remote MCP:
TELESEND_API_KEY='your-secret-key' telesend serve

# Or start REST API only (disabling Remote MCP):
TELESEND_API_KEY='your-secret-key' telesend serve --no-mcp

# Listen on all network interfaces:
TELESEND_API_KEY='your-secret-key' telesend serve --host 0.0.0.0 --port 3100
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
| `GET\|DELETE` | `/history` | List or clear delivery history |
| `GET\|DELETE` | `/history/:id` | Get or delete specific history entry |

### Examples

```sh
# Health check
curl -H "x-api-key: $TELESEND_API_KEY" http://127.0.0.1:3100/health

# Send text
curl -X POST http://127.0.0.1:3100/messages/text \
  -H "x-api-key: $TELESEND_API_KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"ops","payload":{"text":"Hello"}}'

# Send photo via URL
curl -X POST http://127.0.0.1:3100/messages/photo \
  -H "x-api-key: $TELESEND_API_KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"ops","payload":{"photo":{"source":"url","value":"https://example.com/chart.png"},"caption":"Daily chart"}}'

# Upload a file
curl -X POST http://127.0.0.1:3100/messages/photo \
  -H "x-api-key: $TELESEND_API_KEY" \
  -F 'to=ops' \
  -F 'payload={"photo":{"source":"upload","value":"image"},"caption":"Daily chart"}' \
  -F 'image=@./chart.png'
```

> **Note:** Always put an HTTPS reverse proxy in front of Telesend in production. Requests are limited to 52 MiB. Server filesystem paths are rejected in JSON payloads — use `file_id`, `url`, or multipart upload.

## Web UI

Telesend includes a built-in web dashboard to manage and test your setup visually:

- **Bots & Aliases** — Add Telegram bots, map recipient aliases, and configure group thread IDs.
- **Message Composer** — Send test messages, photos, documents, and formatted text directly from your browser.
- **Delivery History** — Browse, filter, and inspect past message delivery logs and statuses.
- **Database Backup & Restore** — Export your database as an SQL file or restore from a previous backup.

To access the Web UI, start the server with a UI password configured:

```sh
TELESEND_UI_PASSWORD='your-secret-password' telesend serve
```

Then visit `http://127.0.0.1:3100/` in your browser.

## Data Storage

By default, Telesend stores its database and configuration in a `data/` folder next to the executable (similar to PocketBase):

| Component | Path |
| --- | --- |
| **Database** | `<binary_dir>/data/telesend.db` |
| **Config** | `<binary_dir>/data/config.json` (or `<binary_dir>/config.json`) |

Set `TELESEND_DATA_DIR` in your environment to override the data folder location.

### Backup & Restore

You can back up your database in two ways:

- **Via Web UI (SQL Export):** Open the Web UI, navigate to the **Settings** panel, and click **Download SQL Export** under Database Backup & Restore. This exports a clean, portable `.sql` dump file of your database that you can save locally and re-import anytime.
- **Via SQLite CLI:** Back up a live database safely without stopping the server:

```sh
sqlite3 data/telesend.db ".backup data/telesend-backup.db"
```

## Development

```sh
make check       # run tests and vet
go test ./...    # run all unit tests
make build       # build standalone binary in dist/telesend
```
