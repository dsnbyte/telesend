# mcp-interface Specification

## Purpose

Defines a stdio MCP interface for outbound delivery while preventing model-initiated access to unauthorized or sensitive local files.

## Requirements

### Requirement: Expose categorized MCP delivery and documentation tools
The MCP server SHALL run over standard input and output and SHALL expose categorized delivery tools covering all supported Telesend content types along with a parameter documentation tool (`get_telegram_parameter_doc`). It SHALL not expose bot or alias administration tools or an unrestricted Telegram method tool.

#### Scenario: Send text through MCP
- **WHEN** an MCP client calls the text tool with a valid recipient and content
- **THEN** the server returns the normalized delivery result

#### Scenario: Retrieve advanced Telegram parameter documentation on demand
- **WHEN** an MCP client calls `get_telegram_parameter_doc` with a supported method or type
- **THEN** the server returns advanced parameter descriptions without requiring full schema bloat in delivery tools

#### Scenario: List aliases with type
- **WHEN** an MCP client calls `list_aliases`
- **THEN** each alias includes `type` `group` or `private` so the client can send to every group alias

### Requirement: Require explicit confirmation before bulk alias delivery
The MCP server SHALL instruct clients to ask for confirmation and wait for an explicit affirmative response before sending when the user intends to send a message to all aliases or all groups.

#### Scenario: Request delivery to every alias
- **WHEN** a user intends to send a message to all aliases or all groups
- **THEN** the client asks for confirmation and waits for an explicit affirmative response before sending

#### Scenario: Confirm delivery to every alias
- **WHEN** the user explicitly confirms the all-alias or all-group delivery
- **THEN** the client may send the message

#### Scenario: Attempt administration through MCP
- **WHEN** an MCP client requests bot or alias mutation
- **THEN** no such tool is available

### Requirement: Accept declared MCP media sources
An MCP media tool SHALL accept `path`, `url`, or `file_id` as an explicit source type where the selected Telegram content type supports that source.

#### Scenario: Send a server-local file
- **WHEN** a media tool receives `source: path` for a permitted local file
- **THEN** the MCP server reads and uploads that file without embedding its bytes in the tool request

#### Scenario: Send URL or Telegram file ID media
- **WHEN** a media tool receives a supported `url` or `file_id` source
- **THEN** the server delivers it without accessing the local filesystem

### Requirement: Restrict local files to allowed roots
For a `path` source, the MCP server SHALL canonicalize the path, require a readable regular file, and require the canonical path to remain under at least one allowed root. The current working directory SHALL be allowed by default; roots from TOML configuration and repeatable `--allow-path` options SHALL be added to it.

#### Scenario: Read a file in an allowed root
- **WHEN** the canonical regular file is inside an allowed root and matches no exclusion
- **THEN** the server permits the upload

#### Scenario: Reject a path outside allowed roots
- **WHEN** the canonical file is outside every allowed root
- **THEN** the tool returns a policy error without reading or uploading the file

#### Scenario: Reject a symlink escape
- **WHEN** a path lexically inside an allowed root resolves through a symlink to a file outside it
- **THEN** the tool rejects the canonical path

### Requirement: Exclude sensitive and application files
The MCP server SHALL enforce non-overridable protected patterns for secrets, Telesend data, and repository metadata. It SHALL also provide built-in default exclusions for source, configuration, dependency, and application asset files, and SHALL allow user-managed TOML include patterns to override only those default exclusions. User-supplied exclude patterns SHALL override include patterns.

#### Scenario: Protected file is requested
- **WHEN** a requested path matches a protected pattern such as an environment file, private key, Telesend database, Telesend configuration, or repository metadata
- **THEN** the server rejects it regardless of configured include patterns

#### Scenario: Default-excluded source is requested
- **WHEN** a requested path matches a built-in source or application-file exclusion and no TOML include pattern matches it
- **THEN** the server rejects it

#### Scenario: Explicitly include a default-excluded report
- **WHEN** a requested path matches a default exclusion and a TOML include pattern but no protected or user exclude pattern
- **THEN** the server permits the file if all other path checks pass

#### Scenario: User exclusion matches
- **WHEN** a requested path matches a configured user exclude pattern
- **THEN** the server rejects it even when an include pattern also matches

### Requirement: Load MCP policy from TOML safely
The MCP server SHALL load policy from `~/.config/telesend/config.toml` by default, SHALL accept an explicit config path at startup, and SHALL apply configuration only after schema validation. A missing file SHALL use safe defaults, while an unreadable or invalid supplied file SHALL prevent MCP startup.

#### Scenario: No config file exists
- **WHEN** MCP starts and the default config file is absent
- **THEN** it starts with the current working directory and built-in exclusions

#### Scenario: Valid policy config exists
- **WHEN** MCP starts with a valid TOML file
- **THEN** it combines configured allow roots and patterns with the built-in policy

#### Scenario: Config is invalid
- **WHEN** the selected TOML file cannot be parsed or fails schema validation
- **THEN** MCP prints an actionable error to standard error and exits nonzero before serving requests
