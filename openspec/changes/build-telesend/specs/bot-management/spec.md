## Purpose

Defines secure registration and lifecycle management for multiple Telegram bot identities, including deterministic default-bot selection.

## ADDED Requirements

### Requirement: Register a verified Telegram bot
The system SHALL require a bot token when registering a bot, call Telegram `getMe` before persisting it, and store the returned Telegram ID, display name, username, and token. The display name SHALL be formed from the returned first and optional last name rather than supplied by the caller.

#### Scenario: Register the first bot
- **WHEN** a valid token for a previously unknown bot is registered while no bots exist
- **THEN** the system stores the Telegram-provided identity and token and marks that bot as the default

#### Scenario: Register an additional bot
- **WHEN** a valid token for a previously unknown bot is registered while a default already exists
- **THEN** the system stores the bot without changing the current default

#### Scenario: Reject an invalid token
- **WHEN** Telegram rejects the token during identity lookup
- **THEN** the system returns an actionable registration error and persists no bot record

#### Scenario: Reject a duplicate bot
- **WHEN** the returned Telegram ID already belongs to a registered bot
- **THEN** the system rejects the duplicate without changing the stored bot or default

### Requirement: Select one default bot
The system SHALL maintain at most one default bot and SHALL allow an operator to make a registered bot the default by its Telegram username.

#### Scenario: Change the default bot
- **WHEN** an operator selects a non-default registered bot as default
- **THEN** the selected bot becomes the sole default in one atomic operation

#### Scenario: Select an unknown bot
- **WHEN** an operator selects a username that is not registered
- **THEN** the system returns a not-found error and preserves the existing default

### Requirement: Inspect bots without exposing credentials
The system SHALL list registered bots with their Telegram ID, display name, username, and default status, and SHALL never include bot tokens in command output, API responses, MCP responses, or application logs.

#### Scenario: List registered bots
- **WHEN** an operator requests the bot list
- **THEN** the system returns identity and default metadata without any token value

### Requirement: Remove registered bots safely
The system SHALL permit removal of a non-default bot, SHALL reject removal of the default while another bot remains, and SHALL permit removal of the only bot so the system returns to having no default.

#### Scenario: Remove a non-default bot
- **WHEN** an operator removes a registered non-default bot
- **THEN** the bot is deleted and the current default is unchanged

#### Scenario: Reject removal of an active default
- **WHEN** an operator attempts to remove the default while another bot exists
- **THEN** the system instructs the operator to select another default and deletes nothing

#### Scenario: Remove the only bot
- **WHEN** an operator removes the default and it is the only registered bot
- **THEN** the bot is deleted and no default bot remains

