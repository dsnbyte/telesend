# cli-interface Specification

## Purpose

Defines the local command-line experience for administering Telesend, sending Telegram messages, and starting its server modes.

## Requirements

### Requirement: Administer bots from the CLI
The CLI SHALL provide commands to add, list, select the default, and remove bots by their Telegram-derived identity. Bot registration SHALL accept the token without requiring a caller-supplied name.

#### Scenario: Add a bot interactively
- **WHEN** an operator runs the bot-add command without placing a token in command arguments
- **THEN** the CLI securely reads the token, verifies the bot, and prints the stored display name, username, and default status

#### Scenario: List bots
- **WHEN** an operator runs the bot-list command
- **THEN** the CLI prints each bot's safe identity metadata and marks the default

### Requirement: Administer aliases from the CLI
The CLI SHALL provide commands to create, list, update, and delete aliases with chat and optional message thread identifiers.

#### Scenario: Add a topic alias
- **WHEN** an operator adds an alias with a chat ID and `--thread-id`
- **THEN** the CLI persists the alias and reports its resolved destination

### Requirement: Send each supported message type from the CLI
The CLI SHALL expose content-specific send commands that accept a direct chat ID or alias, an optional bot username, an optional explicit message thread ID, and the options supported by that content type.

#### Scenario: Send text with defaults
- **WHEN** an operator sends text to an alias without bot or thread options
- **THEN** the CLI resolves the alias and default bot and reports the successful message identity

#### Scenario: Send a local document
- **WHEN** an operator invokes the document command with a readable local path
- **THEN** the CLI uploads the file and reports the successful message identity

#### Scenario: Delivery fails
- **WHEN** a CLI delivery cannot be completed
- **THEN** the CLI prints a concise actionable error to standard error and exits nonzero

### Requirement: Start service modes from the CLI
The CLI SHALL provide separate commands to start the REST server and the stdio MCP server, passing service-specific startup options to those modes.

#### Scenario: Start REST mode
- **WHEN** an operator runs `telesend serve` with valid server configuration
- **THEN** the process starts the authenticated REST service and reports its bound address

#### Scenario: Start MCP mode
- **WHEN** an operator runs `telesend mcp` with valid MCP configuration
- **THEN** the process serves MCP over standard input and output without protocol-breaking log output on standard output

### Requirement: Print human-readable CLI output
The CLI SHALL print human-readable text by default. List commands SHALL render aligned tables. The `--json` flag SHALL print the structured JSON representation of the same result. MCP and REST interfaces SHALL continue to return JSON and SHALL ignore this CLI formatting choice.

#### Scenario: List bots as a table
- **WHEN** an operator runs `telesend bot list` with at least one registered bot
- **THEN** the CLI prints an aligned table of username, name, Telegram ID, and default status without wrapping the rows in JSON

#### Scenario: List aliases as a table
- **WHEN** an operator runs `telesend alias list` with at least one alias
- **THEN** the CLI prints an aligned table of name, chat ID, and thread without wrapping the rows in JSON

#### Scenario: Request JSON output
- **WHEN** an operator passes `--json` to a bot, alias, or message command
- **THEN** the CLI prints pretty-printed JSON for that command's result

#### Scenario: Empty list
- **WHEN** an operator lists bots or aliases and none exist
- **THEN** the CLI prints a concise empty-state message instead of a table or JSON array

### Requirement: Print the installed version
The CLI SHALL print the installed Telesend version to standard output and exit successfully when invoked with `--version` or `-V`.

#### Scenario: Print version with the long flag
- **WHEN** an operator runs `telesend --version`
- **THEN** the CLI prints the installed version and exits 0

#### Scenario: Print version with the short flag
- **WHEN** an operator runs `telesend -V`
- **THEN** the CLI prints the installed version and exits 0

