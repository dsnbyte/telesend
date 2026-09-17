import { createInterface } from "node:readline/promises";
import { Writable } from "node:stream";
import { AppError, redactSecrets } from "./core/errors.ts";
import type { DeliveryResult, RecipientAlias, SafeBot } from "./core/result.ts";
import type { AliasRepository } from "./db/alias-repository.ts";
import type { BotService } from "./services/bot-service.ts";
import type { DeliveryService } from "./services/delivery-service.ts";
import { getOperation, MESSAGE_CATALOG, type MediaSource } from "./telegram/catalog.ts";

const VERSION = "0.1.0";

export interface CliServices {
  aliases: AliasRepository;
  bots: BotService;
  delivery: DeliveryService;
  readSecret?: () => Promise<string>;
  startMcp: (options: { allowPaths: string[]; configPath?: string }) => Promise<void>;
  startRemoteMcp: (options: { hostname?: string; port?: number }) => Promise<void>;
  startRest: (options: { hostname?: string; port?: number }) => Promise<void>;
}

export interface CliIo {
  error(message: string): void;
  log(message: string): void;
}

export async function runCli(
  args: string[],
  services: CliServices,
  io: CliIo = { log: console.log, error: console.error },
): Promise<number> {
  try {
    const json = args.includes("--json");
    const commandArgs = args.filter((arg) => arg !== "--json");
    if (
      commandArgs.length === 0 ||
      commandArgs[0] === "help" ||
      commandArgs[0] === "--help" ||
      commandArgs[0] === "-h"
    ) {
      io.log(help());
      return 0;
    }
    if (commandArgs[0] === "--version" || commandArgs[0] === "-V" || commandArgs[0] === "version") {
      io.log(VERSION);
      return 0;
    }

    switch (commandArgs[0]) {
      case "bot":
        await botCommand(commandArgs.slice(1), services, io, json);
        return 0;
      case "alias":
        aliasCommand(commandArgs.slice(1), services, io, json);
        return 0;
      case "msg":
      case "message":
        await messageCommand(commandArgs.slice(1), services, io, json);
        return 0;
      case "serve":
        await serveCommand(commandArgs.slice(1), services);
        return 0;
      case "mcp":
        await mcpCommand(commandArgs.slice(1), services);
        return 0;
      case "mcp-serve":
        await remoteMcpCommand(commandArgs.slice(1), services);
        return 0;
      default:
        throw new AppError("validation", `Unknown command: ${commandArgs[0]}`);
    }
  } catch (error) {
    io.error(formatCliError(error));
    return 1;
  }
}

async function botCommand(
  args: string[],
  services: CliServices,
  io: CliIo,
  json: boolean,
): Promise<void> {
  switch (args[0]) {
    case "add": {
      if (args.length !== 1) throw new AppError("validation", "Usage: telesend bot add");
      const token = await (services.readSecret ?? readSecret)();
      const bot = await services.bots.register(token);
      print(io, json, bot, formatBot(bot, "Added"));
      return;
    }
    case "list": {
      if (args.length !== 1) throw new AppError("validation", "Usage: telesend bot list");
      const bots = services.bots.list();
      print(io, json, bots, formatBotList(bots));
      return;
    }
    case "default": {
      const bot = services.bots.setDefault(required(args[1], "Bot username is required"));
      print(io, json, bot, `Default bot set to @${bot.username}`);
      return;
    }
    case "remove": {
      const bot = services.bots.remove(required(args[1], "Bot username is required"));
      print(io, json, bot, formatBot(bot, "Removed"));
      return;
    }
    default:
      throw new AppError("validation", "Usage: telesend bot <add|list|default|remove>");
  }
}

function aliasCommand(args: string[], services: CliServices, io: CliIo, json: boolean): void {
  switch (args[0]) {
    case "add": {
      const name = required(args[1], "Alias name is required");
      const chatId = required(args[2], "Chat ID is required");
      const options = parseOptions(args.slice(3));
      const alias = services.aliases.create({
        name,
        chatId,
        ...optionalThread(options),
      });
      print(io, json, alias, formatAlias(alias, "Added"));
      return;
    }
    case "list": {
      const aliases = services.aliases.list();
      print(io, json, aliases, formatAliasList(aliases));
      return;
    }
    case "update": {
      const currentName = required(args[1], "Alias name is required");
      const current = services.aliases.find(currentName);
      if (!current) throw new AppError("not_found", `Alias "${currentName}" was not found`);
      const options = parseOptions(args.slice(2));
      const alias = services.aliases.update(currentName, {
        name: options.name ?? current.name,
        chatId: options["chat-id"] ?? current.chatId,
        messageThreadId:
          options["thread-id"] === undefined
            ? current.messageThreadId
            : parsePositiveInteger(options["thread-id"], "thread-id"),
      });
      print(io, json, alias, formatAlias(alias, "Updated"));
      return;
    }
    case "remove": {
      const alias = services.aliases.remove(required(args[1], "Alias name is required"));
      print(io, json, alias, formatAlias(alias, "Removed"));
      return;
    }
    default:
      throw new AppError("validation", "Usage: telesend alias <add|list|update|remove>");
  }
}

async function messageCommand(
  args: string[],
  services: CliServices,
  io: CliIo,
  json: boolean,
): Promise<void> {
  const type = required(args[0], "Message type is required");
  const operation = getCatalogOperation(type);
  const to = required(args[1], "Recipient alias or chat ID is required");
  const { options, positionals, flags } = splitMessageOptions(args.slice(2));
  const parseMode = options["parse-mode"] && parseTextParseMode(options["parse-mode"]);
  if (parseMode && type !== "text") {
    throw new AppError("validation", "--parse-mode is only supported for text messages");
  }
  let payload: Record<string, unknown>;
  if (options.data) {
    payload = await parseJsonInput(options.data);
  } else if (type === "text") {
    payload = { text: required(positionals.join(" "), "Message text is required") };
  } else if (operation.mediaFields.length === 1 && positionals[0]) {
    payload = { [operation.mediaFields[0] as string]: parseMedia(positionals[0]) };
  } else if (type === "dice") {
    payload = {};
  } else {
    throw new AppError("validation", `Use --data '<json>' to provide fields for ${type}`);
  }
  if (options.caption) payload.caption = options.caption;
  if (parseMode) payload.parse_mode = parseMode;
  if (flags.silent) payload.disable_notification = true;

  const result = await services.delivery.send({
    type,
    to,
    payload,
    ...(options.bot ? { bot: options.bot } : {}),
    ...(options["thread-id"]
      ? { messageThreadId: parsePositiveInteger(options["thread-id"], "thread-id") }
      : {}),
  });
  print(io, json, result, formatDelivery(result));
}

async function serveCommand(args: string[], services: CliServices): Promise<void> {
  const options = parseOptions(args);
  await services.startRest({
    ...(options.host ? { hostname: options.host } : {}),
    ...(options.port ? { port: parsePositiveInteger(options.port, "port") } : {}),
  });
}

async function mcpCommand(args: string[], services: CliServices): Promise<void> {
  const { repeated, values } = parseRepeatedOptions(args);
  await services.startMcp({
    allowPaths: repeated["allow-path"] ?? [],
    ...(values.config ? { configPath: values.config } : {}),
  });
}

async function remoteMcpCommand(args: string[], services: CliServices): Promise<void> {
  const options = parseOptions(args);
  await services.startRemoteMcp({
    ...(options.host ? { hostname: options.host } : {}),
    ...(options.port ? { port: parsePositiveInteger(options.port, "port") } : {}),
  });
}

function help(): string {
  const types = MESSAGE_CATALOG.map(({ type }) => type).join(", ");
  return `Telesend ${VERSION}

Usage:
  telesend [--json] bot add|list|default|remove
  telesend [--json] alias add <name> <chat-id> [--thread-id <id>]
  telesend [--json] alias list|update|remove
  telesend [--json] msg|message <type> <alias|chat-id> [content] [--data <json|@file>] [--bot <username>] [--thread-id <id>] [--silent] [--parse-mode=<html|markdown|md>]
  telesend serve [--host <host>] [--port <port>]
  telesend mcp [--config <path>] [--allow-path <path> ...]
  telesend mcp-serve [--host <host>] [--port <port>]
  telesend --version

Options:
  --json           Print JSON instead of human-readable text
  -V, --version    Print the installed version

Message types:
  ${types}`;
}

function parseMedia(value: string): MediaSource {
  if (value.startsWith("file_id:")) return { source: "file_id", value: value.slice(8) };
  if (/^https?:\/\//i.test(value)) return { source: "url", value };
  return { source: "path", value };
}

async function parseJsonInput(value: string): Promise<Record<string, unknown>> {
  const text = value.startsWith("@") ? await Bun.file(value.slice(1)).text() : value;
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error();
    return parsed as Record<string, unknown>;
  } catch {
    throw new AppError("validation", "--data must contain a JSON object or @file path");
  }
}

function optionalThread(options: Record<string, string>): { messageThreadId?: number } {
  return options["thread-id"]
    ? { messageThreadId: parsePositiveInteger(options["thread-id"], "thread-id") }
    : {};
}

function splitOptions(args: string[]): {
  options: Record<string, string>;
  positionals: string[];
} {
  const options: Record<string, string> = {};
  const positionals: string[] = [];
  for (let index = 0; index < args.length; index += 1) {
    const current = args[index] as string;
    if (!current.startsWith("--")) {
      positionals.push(current);
      continue;
    }
    const value = args[index + 1];
    if (!value || value.startsWith("--"))
      throw new AppError("validation", `${current} requires a value`);
    options[current.slice(2)] = value;
    index += 1;
  }
  return { options, positionals };
}

function splitMessageOptions(args: string[]): {
  options: Record<string, string>;
  positionals: string[];
  flags: Record<"silent", boolean>;
} {
  const options: Record<string, string> = {};
  const positionals: string[] = [];
  const flags = { silent: false };
  for (let index = 0; index < args.length; index += 1) {
    const current = args[index] as string;
    if (!current.startsWith("--")) {
      positionals.push(current);
      continue;
    }
    if (current === "--silent") {
      flags.silent = true;
      continue;
    }
    if (current.startsWith("--parse-mode=")) {
      const value = current.slice("--parse-mode=".length);
      if (!value) throw new AppError("validation", "Use --parse-mode=<html|markdown|md>");
      options["parse-mode"] = value;
      continue;
    }
    if (current === "--parse-mode")
      throw new AppError("validation", "Use --parse-mode=<html|markdown|md>");
    const name = current.slice(2);
    const value = args[index + 1];
    if (!value || value.startsWith("--"))
      throw new AppError("validation", `${current} requires a value`);
    options[name] = value;
    index += 1;
  }
  return { options, positionals, flags };
}

function parseTextParseMode(value: string): "HTML" | "MarkdownV2" {
  if (value === "html") return "HTML";
  if (value === "markdown" || value === "md") return "MarkdownV2";
  throw new AppError("validation", "--parse-mode must be html, markdown, or md");
}

function parseOptions(args: string[]): Record<string, string> {
  const parsed = splitOptions(args);
  if (parsed.positionals.length > 0) {
    throw new AppError("validation", `Unexpected argument: ${parsed.positionals[0]}`);
  }
  return parsed.options;
}

function parseRepeatedOptions(args: string[]): {
  values: Record<string, string>;
  repeated: Record<string, string[]>;
} {
  const values: Record<string, string> = {};
  const repeated: Record<string, string[]> = {};
  for (let index = 0; index < args.length; index += 2) {
    const option = args[index];
    const value = args[index + 1];
    if (!option?.startsWith("--") || !value || value.startsWith("--")) {
      throw new AppError("validation", "MCP options must be --config or --allow-path with a value");
    }
    const key = option.slice(2);
    if (key === "allow-path") {
      repeated[key] ??= [];
      repeated[key].push(value);
    } else values[key] = value;
  }
  return { values, repeated };
}

function parsePositiveInteger(value: string, name: string): number {
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    throw new AppError("validation", `--${name} must be a positive integer`);
  }
  return parsed;
}

function required(value: string | undefined, message: string): string {
  if (!value?.trim()) throw new AppError("validation", message);
  return value.trim();
}

function getCatalogOperation(type: string) {
  try {
    return getOperation(type);
  } catch {
    throw new AppError("validation", `Unsupported message type: ${type}`);
  }
}

function print(io: CliIo, json: boolean, data: unknown, text: string): void {
  io.log(json ? formatJson(data) : text);
}

function formatJson(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function formatBot(bot: SafeBot, action: "Added" | "Removed"): string {
  return [
    `${action} bot @${bot.username}`,
    formatFields([
      ["Name", bot.name],
      ["Telegram ID", bot.telegramId],
      ["Default", yesNo(bot.isDefault)],
    ]),
  ].join("\n");
}

function formatBotList(bots: SafeBot[]): string {
  if (bots.length === 0) return "No bots registered.";
  return formatTable(
    ["USERNAME", "NAME", "ID", "DEFAULT"],
    bots.map((bot) => [bot.username, bot.name, bot.telegramId, yesNo(bot.isDefault)]),
  );
}

function formatAlias(alias: RecipientAlias, action: "Added" | "Updated" | "Removed"): string {
  return [
    `${action} alias ${alias.name}`,
    formatFields([
      ["Chat ID", alias.chatId],
      ["Thread", alias.messageThreadId == null ? "-" : String(alias.messageThreadId)],
    ]),
  ].join("\n");
}

function formatAliasList(aliases: RecipientAlias[]): string {
  if (aliases.length === 0) return "No aliases registered.";
  return formatTable(
    ["NAME", "CHAT ID", "THREAD"],
    aliases.map((alias) => [
      alias.name,
      alias.chatId,
      alias.messageThreadId == null ? "-" : String(alias.messageThreadId),
    ]),
  );
}

function formatDelivery(result: DeliveryResult): string {
  const fields: Array<[string, string]> = [
    ["Bot", `@${result.bot}`],
    ["Chat ID", result.chatId],
  ];
  if (result.messageThreadId !== undefined) {
    fields.push(["Thread", String(result.messageThreadId)]);
  }
  if (result.messageId !== undefined) {
    fields.push(["Message ID", String(result.messageId)]);
  }
  return ["Message sent", formatFields(fields)].join("\n");
}

function formatFields(fields: Array<[string, string]>): string {
  const width = Math.max(...fields.map(([label]) => label.length));
  return fields.map(([label, value]) => `${label.padEnd(width)}  ${value}`).join("\n");
}

function formatTable(headers: string[], rows: string[][]): string {
  const widths = headers.map((header, index) =>
    Math.max(header.length, ...rows.map((row) => (row[index] ?? "").length)),
  );
  const line = (cells: string[]) =>
    cells.map((cell, index) => cell.padEnd(widths[index] as number)).join("  ");
  return [line(headers), line(widths.map((width) => "-".repeat(width))), ...rows.map(line)].join(
    "\n",
  );
}

function yesNo(value: boolean): string {
  return value ? "yes" : "no";
}

function formatCliError(error: unknown): string {
  const message = redactSecrets(error);
  return error instanceof AppError ? `Error: ${message}` : `Unexpected error: ${message}`;
}

async function readSecret(): Promise<string> {
  if (!process.stdin.isTTY) return (await Bun.stdin.text()).trim();
  process.stderr.write("Bot token: ");
  const sink = new Writable({ write: (_chunk, _encoding, callback) => callback() });
  const terminal = createInterface({ input: process.stdin, output: sink, terminal: true });
  try {
    const value = await terminal.question("");
    process.stderr.write("\n");
    return value.trim();
  } finally {
    terminal.close();
  }
}
