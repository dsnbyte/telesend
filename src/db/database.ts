import { Database } from "bun:sqlite";
import { chmodSync, existsSync } from "node:fs";
import { dirname } from "node:path";
import { ensurePrivateDirectory } from "../core/paths.ts";

const MIGRATIONS = [
  `
    CREATE TABLE bots (
      id INTEGER PRIMARY KEY,
      telegram_id TEXT NOT NULL UNIQUE,
      name TEXT NOT NULL,
      username TEXT NOT NULL COLLATE NOCASE UNIQUE,
      token TEXT NOT NULL,
      is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
    CREATE UNIQUE INDEX bots_one_default ON bots(is_default) WHERE is_default = 1;

    CREATE TABLE aliases (
      id INTEGER PRIMARY KEY,
      name TEXT NOT NULL COLLATE NOCASE UNIQUE,
      chat_id TEXT NOT NULL,
      message_thread_id INTEGER NULL CHECK (message_thread_id IS NULL OR message_thread_id > 0),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
  `,
] as const;

export async function openDatabase(path: string): Promise<Database> {
  const existed = existsSync(path);
  if (path !== ":memory:") await ensurePrivateDirectory(dirname(path));

  const database = new Database(path, { create: true, strict: true });
  database.run("PRAGMA foreign_keys = ON");
  database.run("PRAGMA busy_timeout = 5000");
  if (path !== ":memory:") database.run("PRAGMA journal_mode = WAL");
  migrate(database);

  if (path !== ":memory:" && (!existed || process.platform !== "win32")) {
    chmodSync(path, 0o600);
  }
  return database;
}

export function migrate(database: Database): void {
  database.run(`
    CREATE TABLE IF NOT EXISTS schema_migrations (
      version INTEGER PRIMARY KEY,
      applied_at TEXT NOT NULL
    )
  `);
  const applied = database.query("SELECT version FROM schema_migrations").all() as Array<{
    version: number;
  }>;
  const versions = new Set(applied.map((row) => row.version));

  for (const [index, sql] of MIGRATIONS.entries()) {
    const version = index + 1;
    if (versions.has(version)) continue;
    database.transaction(() => {
      database.run(sql);
      database.run("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", [
        version,
        new Date().toISOString(),
      ]);
    })();
  }
}
