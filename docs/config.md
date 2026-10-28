# Telesend Configuration Reference

Telesend can optionally read a JSON configuration file (`config.json`) upon startup.

## File Locations

Telesend searches for `config.json` in the following locations in order of priority:
1. `config.json` alongside the executable (or in the working directory)
2. `<data_dir>/config.json` (by default, `./data/config.json`)
3. Platform-specific config directory:
   - Linux: `~/.config/telesend/config.json`
   - macOS: `~/Library/Preferences/telesend/config.json`
   - Windows: `%APPDATA%\telesend\config.json`

## Configuration Options

```json
{
  "ui": {
    "history_page_size": 20
  },
  "mcp": {
    "files": {
      "allow_paths": ["~/Documents/telesend"],
      "include": ["generated/**/*.json"],
      "exclude": ["**/private/**", "**/*-secret.*"]
    }
  }
}
```

### UI Section (`ui`)

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `history_page_size` | integer | `20` | Number of history records loaded per batch in the Web UI. Clicking **Load More** retrieves the next batch. |

*Note: Environment variable `TELESEND_HISTORY_PAGE_SIZE` can also be used to override this value.*

### MCP Files Section (`mcp.files`)

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `allow_paths` | array of strings | `[]` | Additional directory paths accessible by local file operations in MCP tools (in addition to the working directory). Paths must be absolute or start with `~`. |
| `include` | array of glob patterns | `[]` | Glob patterns that matching files must satisfy to be accessed. |
| `exclude` | array of glob patterns | `[]` | Glob patterns for files or paths that are always denied. |

## Supported Environment Variables

Telesend automatically loads `.env` files from the working directory on startup (see `.env.example`). Below is the complete list of supported environment variables:

| Variable | Required / Optional | Default | Description |
| :--- | :--- | :--- | :--- |
| `TELESEND_DATA_DIR` | Optional | `./data` alongside binary | Path to data directory where the SQLite database (`telesend.db`) and runtime files are stored. |
| `TELESEND_API_KEY_HASH` | Required for REST (or use `TELESEND_API_KEY`) | — | Base64URL-encoded PBKDF2 hash of the REST API key. Generated with `telesend hash-key`. Takes precedence over plaintext key. |
| `TELESEND_API_KEY` | Optional alternative | — | Plaintext REST API key. Hashed in-memory at startup. Useful for local testing. |
| `TELESEND_MCP_PUBLIC_URL` | Required for MCP (unless `--no-mcp`) | — | Public HTTPS origin where remote MCP endpoint is exposed (e.g. `https://telesend.example.com`). |
| `TELESEND_MCP_PASSWORD_HASH` | Required for MCP (or use `TELESEND_MCP_PASSWORD`) | — | Precomputed hash of the MCP password. Generated with `telesend hash-password`. |
| `TELESEND_MCP_PASSWORD` | Optional alternative | — | Plaintext MCP password for local testing. |
| `TELESEND_UI_PASSWORD_HASH` | Required for Web UI (or use `TELESEND_UI_PASSWORD`) | — | Precomputed hash of the Web UI login password. Generated with `telesend hash-password`. |
| `TELESEND_UI_PASSWORD` | Optional alternative | — | Plaintext Web UI login password for local testing. |
| `TELESEND_HISTORY_RETENTION_DAYS` | Optional | `7` | Retention window (in days) for message delivery history before automatic opportunistic pruning. |
| `TELESEND_HISTORY_PAGE_SIZE` | Optional | `20` | Number of history records loaded per batch in the Web UI. Overrides `ui.history_page_size` in `config.json`. |

