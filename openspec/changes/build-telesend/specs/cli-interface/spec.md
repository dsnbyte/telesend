## Purpose

Defines the local command-line experience for administering Telesend, sending Telegram messages, and starting its server modes.

## ADDED Requirements

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

