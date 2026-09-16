import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { chmod, mkdtemp, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { AliasRepository } from "../src/db/alias-repository.ts";
import { BotRepository } from "../src/db/bot-repository.ts";
import { openDatabase } from "../src/db/database.ts";

const databases: Database[] = [];
const roots: string[] = [];

afterEach(async () => {
  for (const database of databases.splice(0)) database.close(false);
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function memory(): Promise<Database> {
  const database = await openDatabase(":memory:");
  databases.push(database);
  return database;
}

describe("database", () => {
  test("applies schema and constraints", async () => {
    const database = await memory();
    const tables = database
      .query("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
      .all() as Array<{ name: string }>;
    expect(tables.map(({ name }) => name)).toContainAllValues([
      "aliases",
      "bots",
      "schema_migrations",
    ]);
    expect(database.query("PRAGMA foreign_keys").get()).toEqual({ foreign_keys: 1 });
    expect(database.query("SELECT version FROM schema_migrations").get()).toEqual({ version: 1 });
  });

  test("creates an owner-only database", async () => {
    const root = await mkdtemp(join(tmpdir(), "telesend-db-"));
    roots.push(root);
    await chmod(root, 0o777);
    const path = join(root, "data", "telesend.db");
    const database = await openDatabase(path);
    databases.push(database);
    if (process.platform !== "win32") {
      expect((await stat(join(root, "data"))).mode & 0o777).toBe(0o700);
      expect((await stat(path)).mode & 0o777).toBe(0o600);
    }
  });
});

describe("BotRepository", () => {
  test("makes only the first bot default and returns safe projections", async () => {
    const repository = new BotRepository(await memory());
    expect(
      repository.add({
        telegramId: "1",
        name: "Primary Bot",
        username: "primary_bot",
        token: "secret-one",
      }),
    ).toEqual({ telegramId: "1", name: "Primary Bot", username: "primary_bot", isDefault: true });
    repository.add({ telegramId: "2", name: "Other", username: "other_bot", token: "secret-two" });
    expect(repository.list().filter((bot) => bot.isDefault)).toHaveLength(1);
    expect(JSON.stringify(repository.list())).not.toContain("secret");
  });

  test("rejects duplicate identity", async () => {
    const repository = new BotRepository(await memory());
    repository.add({ telegramId: "1", name: "One", username: "one_bot", token: "one" });
    expect(() =>
      repository.add({ telegramId: "1", name: "Duplicate", username: "two_bot", token: "two" }),
    ).toThrow("already registered");
  });

  test("switches the sole default atomically", async () => {
    const repository = new BotRepository(await memory());
    repository.add({ telegramId: "1", name: "One", username: "one_bot", token: "one" });
    repository.add({ telegramId: "2", name: "Two", username: "two_bot", token: "two" });
    repository.setDefault("@two_bot");
    expect(repository.list().find((bot) => bot.isDefault)?.username).toBe("two_bot");
  });

  test("protects a default when alternatives remain", async () => {
    const repository = new BotRepository(await memory());
    repository.add({ telegramId: "1", name: "One", username: "one_bot", token: "one" });
    repository.add({ telegramId: "2", name: "Two", username: "two_bot", token: "two" });
    expect(() => repository.remove("one_bot")).toThrow("Select another default");
    repository.setDefault("two_bot");
    expect(repository.remove("one_bot").username).toBe("one_bot");
  });

  test("allows removal of the only bot", async () => {
    const repository = new BotRepository(await memory());
    repository.add({ telegramId: "1", name: "One", username: "one_bot", token: "one" });
    repository.remove("one_bot");
    expect(repository.getDefault()).toBeNull();
  });
});

describe("AliasRepository", () => {
  test("supports CRUD and thread IDs", async () => {
    const repository = new AliasRepository(await memory());
    expect(repository.create({ name: "ops", chatId: "-100123", messageThreadId: 42 })).toEqual({
      name: "ops",
      chatId: "-100123",
      messageThreadId: 42,
    });
    expect(repository.update("ops", { name: "releases", chatId: "-100456" })).toEqual({
      name: "releases",
      chatId: "-100456",
      messageThreadId: null,
    });
    expect(repository.list()).toHaveLength(1);
    repository.remove("releases");
    expect(repository.list()).toHaveLength(0);
  });

  test("rejects duplicates and invalid identifiers", async () => {
    const repository = new AliasRepository(await memory());
    repository.create({ name: "ops", chatId: "-100123" });
    expect(() => repository.create({ name: "OPS", chatId: "2" })).toThrow("already exists");
    expect(() => repository.create({ name: "bad alias", chatId: "2" })).toThrow("Alias must");
    expect(() => repository.create({ name: "valid", chatId: "chat" })).toThrow("Chat ID");
    expect(() => repository.create({ name: "valid", chatId: "2", messageThreadId: 0 })).toThrow(
      "positive integer",
    );
  });
});
