# rest-api Specification

## Purpose

Defines an authenticated internet-facing HTTP interface for message delivery and administration without exposing credentials or server files.

## Requirements

### Requirement: Refuse insecure REST startup
The REST server SHALL read either `TELESEND_API_KEY` or `TELESEND_API_KEY_HASH` at startup and SHALL exit nonzero before binding a listener when both are absent or empty. `TELESEND_API_KEY` SHALL be hashed before use. `TELESEND_API_KEY_HASH` SHALL accept a Bun-compatible password hash or its Base64URL representation and take precedence whenever both variables are set. It SHALL provide `bun run rest:hash-api-key` to interactively generate a Base64URL representation without echoing the entered API key.

#### Scenario: API key is missing
- **WHEN** `telesend serve` starts without a non-empty `TELESEND_API_KEY` or `TELESEND_API_KEY_HASH`
- **THEN** it prints an actionable configuration error, binds no listener, and exits nonzero

#### Scenario: Plaintext API key is present
- **WHEN** `telesend serve` starts with a non-empty `TELESEND_API_KEY` and no `TELESEND_API_KEY_HASH`
- **THEN** it hashes the API key, binds the configured listener, and reports the actual bound address

#### Scenario: API key hash takes precedence
- **WHEN** `telesend serve` starts with both `TELESEND_API_KEY` and a valid `TELESEND_API_KEY_HASH`
- **THEN** it uses `TELESEND_API_KEY_HASH`, binds the configured listener, and reports the actual bound address

#### Scenario: Operator generates an API key hash
- **WHEN** an operator runs `bun run rest:hash-api-key` in an interactive terminal
- **THEN** the command confirms the API key without echoing it and writes a complete `TELESEND_API_KEY_HASH` assignment to standard output

### Requirement: Authenticate every REST request
Every REST endpoint SHALL require an `x-api-key` header whose value verifies against the configured API-key hash, and authentication failures SHALL not reveal the expected value.

#### Scenario: Valid API key
- **WHEN** a request carries the correct `x-api-key`
- **THEN** the server evaluates the requested operation

#### Scenario: Missing or incorrect API key
- **WHEN** a request omits `x-api-key` or supplies an incorrect value
- **THEN** the server returns an unauthorized response without executing the operation

### Requirement: Expose unversioned resource paths
The REST API SHALL expose its message, bot, alias, and health resources without a `/v1` or other version prefix.

#### Scenario: Use a message endpoint
- **WHEN** an authenticated caller posts to `/messages/text`
- **THEN** the server treats it as the text delivery endpoint

#### Scenario: Use a prefixed path
- **WHEN** a caller requests `/v1/messages/text`
- **THEN** the server returns not found

### Requirement: Administer bots and aliases over REST
The REST API SHALL provide authenticated endpoints to register, list, select, and remove bots and to create, list, update, and delete aliases.

#### Scenario: Register a bot through REST
- **WHEN** an authenticated caller posts a valid token to `/bots`
- **THEN** the response contains Telegram-derived safe identity metadata and never echoes the token

#### Scenario: Manage an alias through REST
- **WHEN** an authenticated caller submits a valid alias operation under `/aliases`
- **THEN** the server applies the requested change and returns the safe alias representation

### Requirement: Deliver messages over REST
The REST API SHALL provide one endpoint under `/messages` for each supported content-specific delivery operation and SHALL accept recipient, bot, thread, and content-specific options in an appropriate JSON or multipart request.

#### Scenario: Send a JSON-addressable message
- **WHEN** an authenticated caller submits a valid JSON request to a content-specific endpoint
- **THEN** the server returns the normalized delivery result

#### Scenario: Upload new media
- **WHEN** an authenticated caller submits a valid multipart request with media content
- **THEN** the server uploads that content to Telegram and returns the normalized delivery result

#### Scenario: Submit a server path
- **WHEN** a REST request identifies media as a local server filesystem path
- **THEN** the server rejects the source without reading the path

### Requirement: Return consistent safe HTTP errors
The REST API SHALL map authentication, validation, not-found, conflict, and upstream Telegram failures to stable HTTP status categories and safe structured error bodies.

#### Scenario: Invalid request body
- **WHEN** an authenticated request fails input validation
- **THEN** the server returns a client error with an actionable field-level description and no secrets
