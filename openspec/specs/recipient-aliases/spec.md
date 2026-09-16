# recipient-aliases Specification

## Purpose

Defines reusable recipient aliases that identify Telegram chats and optionally target a specific message thread within a chat.

## Requirements

### Requirement: Manage recipient aliases
The system SHALL allow operators to create, list, update, and delete uniquely named aliases containing a Telegram chat ID and an optional positive integer `message_thread_id`.

#### Scenario: Create a chat alias
- **WHEN** an operator provides an unused alias and a valid chat ID
- **THEN** the system stores the alias with no message thread

#### Scenario: Create a topic alias
- **WHEN** an operator provides an unused alias, a valid chat ID, and a valid message thread ID
- **THEN** the system stores both destination identifiers under that alias

#### Scenario: Reject a duplicate alias
- **WHEN** an operator attempts to create an alias whose name already exists
- **THEN** the system returns a conflict and leaves the existing alias unchanged

#### Scenario: Update an alias destination
- **WHEN** an operator changes the chat ID or message thread ID of an existing alias
- **THEN** subsequent resolution uses the updated destination

#### Scenario: Delete an alias
- **WHEN** an operator deletes an existing alias
- **THEN** the alias can no longer be resolved

### Requirement: Resolve aliases and direct chat IDs
The system SHALL accept either a stored alias or a direct Telegram chat ID as a message destination. An explicitly supplied message thread ID SHALL override the thread stored by an alias.

#### Scenario: Resolve an alias with a thread
- **WHEN** a delivery targets an alias containing a chat ID and message thread ID and supplies no explicit thread
- **THEN** the system uses both stored identifiers for the Telegram request

#### Scenario: Override an alias thread
- **WHEN** a delivery targets an alias and supplies an explicit message thread ID
- **THEN** the system uses the alias chat ID and the explicit thread ID

#### Scenario: Use a direct chat ID
- **WHEN** a delivery supplies a valid direct chat ID
- **THEN** the system uses it without requiring a stored alias

#### Scenario: Reject an unknown alias
- **WHEN** a non-chat-ID destination does not match a stored alias
- **THEN** the system returns a recipient-not-found error without contacting Telegram

