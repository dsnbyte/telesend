import type { Database } from "bun:sqlite";
import { AppError } from "../core/errors.ts";
import type { RecipientAlias } from "../core/result.ts";

interface AliasRow {
  name: string;
  chat_id: string;
  message_thread_id: number | null;
}

export interface AliasInput {
  name: string;
  chatId: string;
  messageThreadId?: number | null;
}

const ALIAS_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
const CHAT_ID_PATTERN = /^-?\d+$/;

function validate(input: AliasInput): Required<AliasInput> {
  const name = input.name.trim();
  const chatId = input.chatId.trim();
  const messageThreadId = input.messageThreadId ?? null;
  if (!ALIAS_PATTERN.test(name)) {
    throw new AppError(
      "validation",
      "Alias must contain only letters, numbers, dot, dash, or underscore",
    );
  }
  if (!CHAT_ID_PATTERN.test(chatId)) {
    throw new AppError("validation", "Chat ID must be a signed decimal integer");
  }
  if (
    messageThreadId !== null &&
    (!Number.isSafeInteger(messageThreadId) || messageThreadId <= 0)
  ) {
    throw new AppError("validation", "Message thread ID must be a positive integer");
  }
  return { name, chatId, messageThreadId };
}

function fromRow(row: AliasRow): RecipientAlias {
  return { name: row.name, chatId: row.chat_id, messageThreadId: row.message_thread_id };
}

export class AliasRepository {
  constructor(private readonly database: Database) {}

  create(input: AliasInput): RecipientAlias {
    const value = validate(input);
    const now = new Date().toISOString();
    try {
      this.database.run(
        `INSERT INTO aliases (name, chat_id, message_thread_id, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?)`,
        [value.name, value.chatId, value.messageThreadId, now, now],
      );
    } catch (error) {
      if (String(error).includes("UNIQUE constraint failed")) {
        throw new AppError("conflict", `Alias "${value.name}" already exists`);
      }
      throw error;
    }
    return value;
  }

  list(): RecipientAlias[] {
    const rows = this.database
      .query("SELECT name, chat_id, message_thread_id FROM aliases ORDER BY name COLLATE NOCASE")
      .all() as AliasRow[];
    return rows.map(fromRow);
  }

  find(name: string): RecipientAlias | null {
    const row = this.database
      .query("SELECT name, chat_id, message_thread_id FROM aliases WHERE name = ? COLLATE NOCASE")
      .get(name.trim()) as AliasRow | null;
    return row ? fromRow(row) : null;
  }

  update(currentName: string, input: AliasInput): RecipientAlias {
    if (!this.find(currentName)) {
      throw new AppError("not_found", `Alias "${currentName}" was not found`);
    }
    const value = validate(input);
    try {
      this.database.run(
        `UPDATE aliases SET name = ?, chat_id = ?, message_thread_id = ?, updated_at = ?
         WHERE name = ? COLLATE NOCASE`,
        [value.name, value.chatId, value.messageThreadId, new Date().toISOString(), currentName],
      );
    } catch (error) {
      if (String(error).includes("UNIQUE constraint failed")) {
        throw new AppError("conflict", `Alias "${value.name}" already exists`);
      }
      throw error;
    }
    return value;
  }

  remove(name: string): RecipientAlias {
    const existing = this.find(name);
    if (!existing) throw new AppError("not_found", `Alias "${name}" was not found`);
    this.database.run("DELETE FROM aliases WHERE name = ? COLLATE NOCASE", [name.trim()]);
    return existing;
  }
}
