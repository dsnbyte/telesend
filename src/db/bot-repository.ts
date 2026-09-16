import type { Database } from "bun:sqlite";
import { AppError } from "../core/errors.ts";
import type { SafeBot } from "../core/result.ts";

export interface StoredBot extends SafeBot {
  token: string;
}

export interface VerifiedBot {
  telegramId: string;
  name: string;
  username: string;
  token: string;
}

interface BotRow {
  telegram_id: string;
  name: string;
  username: string;
  token: string;
  is_default: number;
}

function normalizeUsername(username: string): string {
  return username.trim().replace(/^@/, "");
}

function rowToStored(row: BotRow): StoredBot {
  return {
    telegramId: row.telegram_id,
    name: row.name,
    username: row.username,
    token: row.token,
    isDefault: row.is_default === 1,
  };
}

function safe(bot: StoredBot): SafeBot {
  const { token: _token, ...projection } = bot;
  return projection;
}

export class BotRepository {
  constructor(private readonly database: Database) {}

  add(bot: VerifiedBot): SafeBot {
    if (!bot.telegramId || !bot.name.trim() || !bot.username.trim() || !bot.token.trim()) {
      throw new AppError("validation", "Telegram bot identity is incomplete");
    }
    const transaction = this.database.transaction(() => {
      const count = this.database.query("SELECT COUNT(*) AS count FROM bots").get() as {
        count: number;
      };
      const now = new Date().toISOString();
      this.database.run(
        `INSERT INTO bots
          (telegram_id, name, username, token, is_default, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        [
          bot.telegramId,
          bot.name.trim(),
          normalizeUsername(bot.username),
          bot.token,
          count.count === 0 ? 1 : 0,
          now,
          now,
        ],
      );
      return this.getRequired(bot.username);
    });

    try {
      return safe(transaction.immediate());
    } catch (error) {
      if (String(error).includes("UNIQUE constraint failed")) {
        throw new AppError("conflict", "This Telegram bot is already registered");
      }
      throw error;
    }
  }

  list(): SafeBot[] {
    return this.rows().map((row) => safe(rowToStored(row)));
  }

  find(username: string): StoredBot | null {
    const row = this.database
      .query(
        `SELECT telegram_id, name, username, token, is_default
         FROM bots WHERE username = ? COLLATE NOCASE`,
      )
      .get(normalizeUsername(username)) as BotRow | null;
    return row ? rowToStored(row) : null;
  }

  getDefault(): StoredBot | null {
    const row = this.database
      .query("SELECT telegram_id, name, username, token, is_default FROM bots WHERE is_default = 1")
      .get() as BotRow | null;
    return row ? rowToStored(row) : null;
  }

  setDefault(username: string): SafeBot {
    const transaction = this.database.transaction(() => {
      const bot = this.getRequired(username);
      this.database.run("UPDATE bots SET is_default = 0 WHERE is_default = 1");
      this.database.run("UPDATE bots SET is_default = 1, updated_at = ? WHERE username = ?", [
        new Date().toISOString(),
        bot.username,
      ]);
      return safe({ ...bot, isDefault: true });
    });
    return transaction.immediate();
  }

  remove(username: string): SafeBot {
    const transaction = this.database.transaction(() => {
      const bot = this.getRequired(username);
      const count = this.database.query("SELECT COUNT(*) AS count FROM bots").get() as {
        count: number;
      };
      if (bot.isDefault && count.count > 1) {
        throw new AppError(
          "conflict",
          "Select another default bot before removing the current default",
        );
      }
      this.database.run("DELETE FROM bots WHERE username = ? COLLATE NOCASE", [bot.username]);
      return safe(bot);
    });
    return transaction.immediate();
  }

  private getRequired(username: string): StoredBot {
    const bot = this.find(username);
    if (!bot) throw new AppError("not_found", `Bot @${normalizeUsername(username)} was not found`);
    return bot;
  }

  private rows(): BotRow[] {
    return this.database
      .query(
        `SELECT telegram_id, name, username, token, is_default
         FROM bots ORDER BY is_default DESC, username COLLATE NOCASE`,
      )
      .all() as BotRow[];
  }
}
