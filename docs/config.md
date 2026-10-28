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
