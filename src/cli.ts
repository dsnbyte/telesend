import { createInterface } from "node:readline/promises";
import { Writable } from "node:stream";
import { AppError, redactSecrets } from "./core/errors.ts";
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
    if (args.length === 0 || args[0] === "help" || args[0] === "--help" || args[0] === "-h") {
      io.log(help());
      return 0;
    }
    if (args[0] === "--version" || args[0] === "version") {
      io.log(VERSION);
      return 0;
    }

    switch (args[0]) {
      case "bot":
        await botCommand(args.slice(1), services, io);
        return 0;
      case "alias":
        aliasCommand(args.slice(1), services, io);
        return 0;
      case "msg":
      case "message":
        await messageCommand(args.slice(1), services, io);
        return 0;
      case "serve":
        await serveCommand(args.slice(1), services);
        return 0;
      case "mcp":
        await mcpCommand(args.slice(1), services);
        return 0;
      default:
        throw new AppError("validation", `Unknown command: ${args[0]}`);
    }
  } catch (error) {
    io.error(formatCliError(error));
    return 1;
  }
}

async function botCommand(args: string[], services: CliServices, io: CliIo): Promise<void> {
  switch (args[0]) {
    case "add": {
      if (args.length !== 1) throw new AppError("validation", "Usage: telesend bot add");
      const token = await (services.readSecret ?? readSecret)();
      io.log(formatJson(await services.bots.register(token)));
      return;
    }
    case "list":
      if (args.length !== 1) throw new AppError("validation", "Usage: telesend bot list");
      io.log(formatJson(services.bots.list()));
      return;
    case "default":
      io.log(formatJson(services.bots.setDefault(required(args[1], "Bot username is required"))));
      return;
    case "remove":
      io.log(formatJson(services.bots.remove(required(args[1], "Bot username is required"))));
      return;
    default:
      throw new AppError("validation", "Usage: telesend bot <add|list|default|remove>");
  }
}

function aliasCommand(args: string[], services: CliServices, io: CliIo): void {
  switch (args[0]) {
    case "add": {
      const name = required(args[1], "Alias name is required");
      const chatId = required(args[2], "Chat ID is required");
      const options = parseOptions(args.slice(3));
      io.log(
        formatJson(
          services.aliases.create({
            name,
            chatId,
            ...optionalThread(options),
          }),
        ),
      );
      return;
    }
    case "list":
      io.log(formatJson(services.aliases.list()));
      return;
    case "update": {
      const currentName = required(args[1], "Alias name is required");
      const current = services.aliases.find(currentName);
      if (!current) throw new AppError("not_found", `Alias "${currentName}" was not found`);
      const options = parseOptions(args.slice(2));
      io.log(
        formatJson(
          services.aliases.update(currentName, {
            name: options.name ?? current.name,
            chatId: options["chat-id"] ?? current.chatId,
            messageThreadId:
              options["thread-id"] === undefined
                ? current.messageThreadId
                : parsePositiveInteger(options["thread-id"], "thread-id"),
          }),
        ),
      );
      return;
    }
    case "remove":
      io.log(formatJson(services.aliases.remove(required(args[1], "Alias name is required"))));
      return;
    default:
      throw new AppError("validation", "Usage: telesend alias <add|list|update|remove>");
  }
}

async function messageCommand(args: string[], services: CliServices, io: CliIo): Promise<void> {
  const type = required(args[0], "Message type is required");
  const operation = getCatalogOperation(type);
  const to = required(args[1], "Recipient alias or chat ID is required");
  const { options, positionals } = splitOptions(args.slice(2));
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

  const result = await services.delivery.send({
    type,
    to,
    payload,
    ...(options.bot ? { bot: options.bot } : {}),
    ...(options["thread-id"]
      ? { messageThreadId: parsePositiveInteger(options["thread-id"], "thread-id") }
      : {}),
  });
  io.log(formatJson(result));
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

function help(): string {
  const types = MESSAGE_CATALOG.map(({ type }) => type).join(", ");
  return `Telesend ${VERSION}

Usage:
  telesend bot add|list|default|remove
  telesend alias add <name> <chat-id> [--thread-id <id>]
  telesend alias list|update|remove
  telesend msg|message <type> <alias|chat-id> [content] [--data <json|@file>] [--bot <username>] [--thread-id <id>]
  telesend serve [--host <host>] [--port <port>]
  telesend mcp [--config <path>] [--allow-path <path> ...]

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

function formatJson(value: unknown): string {
  return JSON.stringify(value, null, 2);
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
