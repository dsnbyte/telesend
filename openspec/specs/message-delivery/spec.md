# message-delivery Specification

## Purpose

Defines consistent outbound Telegram message delivery across transports, including bot selection, recipient resolution, media sources, and results.

## Requirements

### Requirement: Select a bot for delivery
The system SHALL use the default bot when no bot is specified and SHALL allow a caller to select a registered bot by Telegram username.

#### Scenario: Send with the default bot
- **WHEN** a delivery request omits the bot and a default exists
- **THEN** the system sends through the default bot

#### Scenario: Send with an explicit bot
- **WHEN** a delivery request names a registered bot username
- **THEN** the system sends through that bot without changing the default

#### Scenario: No default bot exists
- **WHEN** a delivery request omits the bot and no default exists
- **THEN** the system returns an actionable bot-selection error without contacting Telegram

#### Scenario: Explicit bot is unknown
- **WHEN** a delivery request names an unregistered bot username
- **THEN** the system returns a bot-not-found error without contacting Telegram

### Requirement: Support Telegram message content types explicitly
The system SHALL provide a dedicated operation for every Telegram Bot API message-sending content type supported by the Telesend release. The release SHALL document and test its supported operation catalog, and SHALL NOT expose an unrestricted generic Telegram method proxy.

#### Scenario: Send a supported content type
- **WHEN** a caller supplies the required fields for a documented content-specific operation
- **THEN** the system invokes the corresponding Telegram method with the provided supported options

#### Scenario: Required content field is missing
- **WHEN** a caller omits a required field for the selected content type
- **THEN** the system returns a validation error before contacting Telegram

#### Scenario: Attempt a generic Telegram call
- **WHEN** a caller requests an arbitrary Telegram method that is not an explicit Telesend operation
- **THEN** the system rejects the request because generic pass-through is not supported

### Requirement: Accept appropriate media sources
For media operations that accept uploaded content, the shared delivery service SHALL support Telegram `file_id` values and public URLs where Telegram permits them, and SHALL support a validated local file supplied by the CLI or MCP transports. The REST transport SHALL accept new file content through multipart upload and SHALL NOT interpret client-provided server filesystem paths.

#### Scenario: Reuse a Telegram file
- **WHEN** a media request identifies its source as `file_id`
- **THEN** the system passes the value to Telegram without reading a local file

#### Scenario: Send media by URL
- **WHEN** a media request identifies its source as `url`
- **THEN** the system passes the URL to Telegram for a content type that permits URL delivery

#### Scenario: Upload a validated local file
- **WHEN** the CLI or MCP service supplies an allowed readable local file
- **THEN** the system uploads it to Telegram using multipart form data

#### Scenario: Reject an unsupported source for a content type
- **WHEN** a source form is not permitted for the selected Telegram content type or transport
- **THEN** the system returns a validation error without attempting an invalid delivery

### Requirement: Return a normalized delivery result
The system SHALL return the Telegram message identity, destination identity, and selected bot identity after successful delivery, while preserving content-specific result data needed by callers.

#### Scenario: Delivery succeeds
- **WHEN** Telegram accepts a message request
- **THEN** the caller receives a success result containing the message, destination, and bot identifiers without credentials

### Requirement: Report delivery failures safely
The system SHALL distinguish local validation and resolution failures from Telegram delivery failures and SHALL never expose bot tokens, API keys, or credential-bearing Telegram URLs in errors or logs.

#### Scenario: Telegram rejects delivery
- **WHEN** Telegram returns an error for a valid outbound request
- **THEN** the system returns a sanitized actionable failure associated with the requested operation

#### Scenario: Secret appears in an upstream error
- **WHEN** upstream error data contains a configured secret
- **THEN** the system redacts the secret before logging or returning the error

