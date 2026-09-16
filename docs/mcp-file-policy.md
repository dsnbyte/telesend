# MCP file policy

Local paths in MCP tool calls are read by the machine running `telesend mcp`. Telesend permits the startup working directory by default. Add roots with `--allow-path` or `mcp.files.allow_paths` in `config.toml`.

Policy evaluation is fail-closed:

1. The canonical path must be a readable regular file under an allowed root.
2. Environment files, private keys, repository metadata, and the active Telesend database and config are always protected.
3. User `exclude` patterns always deny a match.
4. Source, project config, dependencies, and conventional `src`, `config`, and `assets` directories are denied by default.
5. User `include` patterns can override only the default exclusions.

Patterns use Bun glob syntax and match paths relative to each allowed root. Symlinks are resolved before containment and pattern checks.
