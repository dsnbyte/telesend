import { McpServer } from "@modelcontextprotocol/server";
import { StdioServerTransport } from "@modelcontextprotocol/server/stdio";
import { z } from "zod";
import type { Application } from "../app.ts";
import { AppError, toAppError } from "../core/errors.ts";
import { MESSAGE_CATALOG, type MessageOperation, type MessageType } from "../telegram/catalog.ts";
import { loadMcpConfig } from "./config.ts";
import { FilePolicy } from "./file-policy.ts";

export interface McpOptions {
  allowPaths: string[];
  configPath?: string;
}

interface McpToolPolicy {
  authorizePayload(value: unknown): Promise<unknown>;
  canRead: boolean;
  canSend: boolean;
  includePaths: boolean;
}

const SERVER_INSTRUCTIONS = [
  "Send Telegram messages through registered bots.",
  "Use send_text for ordinary chat; other send_* tools match their content type.",
  "Every send_* tool accepts payload.disable_notification: true for silent delivery.",
  "to: chat ID, @username, or an alias from list_aliases.",
  "Omit bot to use the default from list_bots.",
].join("\n");

const MCP_DESCRIPTIONS = {
  text: 'Send a Telegram text message. In payload, use parse_mode: "HTML" or "MarkdownV2" for formatting.',
  "message-draft": "Send a streaming text draft, not a regular chat.",
  "rich-message": "Send a rich/structured Telegram message.",
  "rich-message-draft": "Send a streaming rich-message draft.",
  animation: "Send a Telegram GIF or animation.",
  audio: "Send a Telegram audio track.",
  document: "Send a Telegram document or file.",
  "live-photo": "Send a Telegram live photo.",
  photo: "Send a Telegram photo.",
  sticker: "Send a Telegram sticker.",
  video: "Send a Telegram video.",
  "video-note": "Send a round Telegram video note.",
  voice: "Send a Telegram voice note.",
  "paid-media": "Send paid Telegram media for stars.",
  "media-group": "Send a Telegram media album.",
  location: "Send a geographic location.",
  venue: "Send a venue with location, title, and address.",
  contact: "Send a phone contact.",
  poll: "Send a Telegram poll.",
  checklist: "Send a Telegram business checklist.",
  dice: "Send a Telegram dice animation.",
  invoice: "Send a Telegram invoice.",
  game: "Send an HTML5 Telegram game.",
} as const satisfies Record<MessageType, string>;

export function createMcpServer(app: Application, filePolicy: FilePolicy): McpServer {
  return createToolServer(app, {
    authorizePayload: (value) => authorizeMcpPayload(value, filePolicy),
    canRead: true,
    canSend: true,
    includePaths: true,
  });
}

export function createRemoteMcpServer(app: Application, scopes: readonly string[]): McpServer {
  return createToolServer(app, {
    authorizePayload: authorizeRemoteMcpPayload,
    canRead: scopes.includes("mcp:read"),
    canSend: scopes.includes("mcp:send"),
    includePaths: false,
  });
}

function createToolServer(app: Application, policy: McpToolPolicy): McpServer {
  const server = new McpServer(
    { name: "telesend", version: "0.1.0" },
    { instructions: SERVER_INSTRUCTIONS },
  );

  server.registerTool(
    "list_aliases",
    {
      title: "List aliases",
      description: "List recipient aliases that can be used as `to`.",
      inputSchema: z.object({}),
      annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true },
    },
    () =>
      policy.canRead
        ? jsonResult({ aliases: app.aliases.list() })
        : toolError(new AppError("unauthorized", "MCP scope mcp:read is required")),
  );
  server.registerTool(
    "list_bots",
    {
      title: "List bots",
      description: "List registered bots and which one is the default.",
      inputSchema: z.object({}),
      annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true },
    },
    () =>
      policy.canRead
        ? jsonResult({ bots: app.bots.list() })
        : toolError(new AppError("unauthorized", "MCP scope mcp:read is required")),
  );

  for (const operation of MESSAGE_CATALOG) {
    server.registerTool(
      toolName(operation.type),
      {
        title: `Send ${operation.description}`,
        description: MCP_DESCRIPTIONS[operation.type as keyof typeof MCP_DESCRIPTIONS],
        annotations: {
          readOnlyHint: false,
          destructiveHint: false,
          idempotentHint: false,
          openWorldHint: true,
        },
        inputSchema: z.object({
          to: z.string().min(1).describe("Chat ID, @username, or alias from list_aliases"),
          bot: z
            .string()
            .min(1)
            .optional()
            .describe("Bot username; default from list_bots if omitted"),
          messageThreadId: z.number().int().positive().optional(),
          payload: payloadSchema(operation, policy.includePaths),
        }),
      },
      async ({ to, bot, messageThreadId, payload }) => {
        try {
          if (!policy.canSend) {
            throw new AppError("unauthorized", "MCP scope mcp:send is required");
          }
          const safePayload = (await policy.authorizePayload(payload)) as Record<string, unknown>;
          return jsonResult(
            (await app.delivery.send({
              type: operation.type,
              to,
              ...(bot === undefined ? {} : { bot }),
              ...(messageThreadId === undefined ? {} : { messageThreadId }),
              payload: safePayload,
            })) as unknown as Record<string, unknown>,
          );
        } catch (error) {
          return toolError(error);
        }
      },
    );
  }
  return server;
}

export async function startMcpServer(app: Application, options: McpOptions): Promise<void> {
  const config = await loadMcpConfig(
    options.configPath === undefined ? {} : { configPath: options.configPath },
  );
  const filePolicy = await FilePolicy.create({
    cwd: process.cwd(),
    configuredRoots: config.files.allowPaths,
    cliRoots: options.allowPaths,
    include: config.files.include,
    exclude: config.files.exclude,
    protectedPaths: [config.configPath, app.databasePath],
  });
  await createMcpServer(app, filePolicy).connect(new StdioServerTransport());
}

export async function authorizeMcpPayload(value: unknown, policy: FilePolicy): Promise<unknown> {
  if (Array.isArray(value)) {
    return Promise.all(value.map((item) => authorizeMcpPayload(item, policy)));
  }
  if (!value || typeof value !== "object") return value;

  const record = value as Record<string, unknown>;
  if ("source" in record) {
    if (!isMcpSource(record.source)) {
      throw new AppError("validation", "MCP media source must be path, url, or file_id");
    }
    if (typeof record.value !== "string" || record.value.length === 0) {
      throw new AppError("validation", `MCP media source ${record.source} requires a value`);
    }
    if (record.source === "path") {
      return { source: "path", value: await policy.authorize(record.value) };
    }
    return { source: record.source, value: record.value };
  }

  return Object.fromEntries(
    await Promise.all(
      Object.entries(record).map(async ([key, item]) => [
        key,
        await authorizeMcpPayload(item, policy),
      ]),
    ),
  );
}

export async function authorizeRemoteMcpPayload(value: unknown): Promise<unknown> {
  if (Array.isArray(value)) return Promise.all(value.map(authorizeRemoteMcpPayload));
  if (!value || typeof value !== "object") return value;

  const record = value as Record<string, unknown>;
  if ("source" in record) {
    if (!isMcpSource(record.source)) {
      throw new AppError("validation", "MCP media source must be url or file_id");
    }
    if (typeof record.value !== "string" || record.value.length === 0) {
      throw new AppError("validation", `MCP media source ${record.source} requires a value`);
    }
    if (record.source === "path") {
      throw new AppError("filesystem_policy", "Remote MCP cannot use server filesystem paths");
    }
    return { source: record.source, value: record.value };
  }

  return Object.fromEntries(
    await Promise.all(
      Object.entries(record).map(async ([key, item]) => [
        key,
        await authorizeRemoteMcpPayload(item),
      ]),
    ),
  );
}

export function toolName(type: string): string {
  return `send_${type.replaceAll("-", "_")}`;
}

function payloadSchema(operation: MessageOperation, includePaths: boolean) {
  return z
    .object(
      Object.fromEntries(
        operation.requiredFields.map((field) => [
          field,
          payloadField(operation, field, includePaths),
        ]),
      ),
    )
    .loose();
}

function payloadField(operation: MessageOperation, field: string, includePaths: boolean) {
  if (operation.mediaFields.includes(field)) {
    if (field === "media") {
      return z.array(
        z
          .object({
            type: z.string().min(1),
            media: mcpMediaSource(operation.allowUrl, includePaths),
          })
          .loose(),
      );
    }
    return mcpMediaSource(operation.allowUrl, includePaths);
  }
  switch (field) {
    case "draft_id":
    case "star_count":
      return z.number().int().positive();
    case "latitude":
    case "longitude":
      return z.number();
    case "options":
      return z.array(z.object({ text: z.string().min(1) }).loose());
    case "prices":
      return z.array(
        z
          .object({
            label: z.string().min(1),
            amount: z.number().int(),
          })
          .loose(),
      );
    case "rich_message":
    case "checklist":
      return z.object({}).loose();
    default:
      return z.string().min(1);
  }
}

function mcpMediaSource(allowUrl: boolean, includePaths: boolean) {
  const sources = [
    ...(includePaths ? (["path"] as const) : []),
    ...(allowUrl ? (["url"] as const) : []),
    "file_id" as const,
  ];
  return z.object({
    source: z.enum(sources),
    value: z.string().min(1),
  });
}

function jsonResult(data: Record<string, unknown>) {
  return {
    content: [{ type: "text" as const, text: JSON.stringify(data) }],
    structuredContent: data,
  };
}

function toolError(error: unknown) {
  const safe = toAppError(error);
  return {
    isError: true,
    content: [{ type: "text" as const, text: `${safe.code}: ${safe.message}` }],
  };
}

function isMcpSource(value: unknown): value is "file_id" | "path" | "url" {
  return value === "file_id" || value === "path" || value === "url";
}
