import { McpServer } from "@modelcontextprotocol/server";
import { StdioServerTransport } from "@modelcontextprotocol/server/stdio";
import { z } from "zod";
import type { Application } from "../app.ts";
import { AppError, toAppError } from "../core/errors.ts";
import { getOperation } from "../telegram/catalog.ts";
import { loadMcpConfig } from "./config.ts";
import { getMethodDoc, KNOWN_QUERIES } from "./docs.ts";
import { FilePolicy } from "./file-policy.ts";
import { ICON_DATA_URL } from "./icon.ts";

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
  "Pick the send_* tool that matches the content: send_text, send_rich_message, send_media (photo/video/audio/document/sticker/animation/...), send_media_group, send_location (add title+address for a venue), send_interactive (poll/checklist/dice/game), send_invoice, or send_contact.",
  "list_aliases and list_bots are lookup helpers.",
  "Every send_* tool accepts payload.disable_notification: true for silent delivery.",
  "to: chat ID, @username, or alias from list_aliases. Omit bot to use the default.",
  "Call get_telegram_parameter_doc only for advanced Telegram options not present in the standard tool parameters.",
].join("\n");

// ─── Shared schema helpers ────────────────────────────────────────────────────

function mcpMediaSource(allowUrl: boolean, includePaths: boolean) {
  const sources = [
    ...(includePaths ? (["path"] as const) : []),
    ...(allowUrl ? (["url"] as const) : []),
    "file_id" as const,
  ];
  const description = includePaths
    ? "Use a policy-approved path, a public URL where supported, or a Telegram file_id."
    : allowUrl
      ? "Use a public HTTPS URL reachable by Telegram or a Telegram file_id. Chat attachments and generated files cannot be uploaded directly through remote MCP."
      : "Use a Telegram file_id. Chat attachments and generated files cannot be uploaded directly through remote MCP.";
  return z.object({
    source: z.enum(sources).describe(description),
    value: z.string().min(1).describe(description),
  });
}

/** Common top-level fields shared by all send_* tools */
function envelopeSchema<T extends z.ZodRawShape>(payloadShape: T) {
  return z.object({
    to: z.string().min(1).describe("Chat ID, @username, or alias from list_aliases"),
    bot: z.string().min(1).optional().describe("Bot username; default from list_bots if omitted"),
    messageThreadId: z.number().int().positive().optional(),
    payload: z.object(payloadShape).loose(),
  });
}

// ─── Media type routing helpers ───────────────────────────────────────────────

/** Map send_media `type` field values to catalog type names */
const MEDIA_TYPE_TO_CATALOG: Record<string, string> = {
  photo: "photo",
  video: "video",
  animation: "animation",
  audio: "audio",
  document: "document",
  sticker: "sticker",
  voice: "voice",
  video_note: "video-note",
  live_photo: "live-photo",
};

/** For most types the media field name equals the type, except for these */
const MEDIA_TYPE_TO_FIELD: Record<string, string> = {
  video_note: "video_note",
  live_photo: "live_photo",
};

/** Types that do not allow URL sources */
const MEDIA_TYPES_NO_URL = new Set(["video_note", "live_photo"]);

// ─── Payload transform helpers ────────────────────────────────────────────────

/**
 * Remap send_media's unified `file` field to the type-specific field name
 * expected by the delivery service (e.g. file → video, file → audio).
 */
function remapMediaPayload(
  type: string,
  payload: Record<string, unknown>,
): Record<string, unknown> {
  const { file, ...rest } = payload;
  const fieldName = MEDIA_TYPE_TO_FIELD[type] ?? type;
  return { ...rest, [fieldName]: file };
}

// ─── Server factories ─────────────────────────────────────────────────────────

export function createMcpServer(app: Application, filePolicy: FilePolicy): McpServer {
  return createToolServer(app, {
    authorizePayload: (value) => authorizeMcpPayload(value, filePolicy),
    canRead: true,
    canSend: true,
    includePaths: true,
  });
}

export function createRemoteMcpServer(
  app: Application,
  scopes: readonly string[],
  iconUrl?: string,
): McpServer {
  return createToolServer(
    app,
    {
      authorizePayload: authorizeRemoteMcpPayload,
      canRead: scopes.includes("mcp:read"),
      canSend: scopes.includes("mcp:send"),
      includePaths: false,
    },
    iconUrl,
  );
}

// ─── Core server builder ──────────────────────────────────────────────────────

function createToolServer(app: Application, policy: McpToolPolicy, iconUrl?: string): McpServer {
  const server = new McpServer(
    {
      name: "telesend",
      version: "0.1.0",
      description: "Send Telegram bot messages via CLI, MCP, and REST",
      icons: [
        {
          src: iconUrl ?? ICON_DATA_URL,
          mimeType: "image/png",
          sizes: ["512x512"],
        },
      ],
    },
    { instructions: SERVER_INSTRUCTIONS },
  );

  // ── Discovery tools ─────────────────────────────────────────────────────────

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

  // ── Documentation tool ──────────────────────────────────────────────────────

  server.registerTool(
    "get_telegram_parameter_doc",
    {
      title: "Get Telegram parameter docs",
      description:
        "Return advanced parameter documentation for a Telegram Bot API method or Telesend message type. " +
        "ONLY call this tool if you need advanced parameters or customization options not available in the send_* tool schemas. " +
        "DO NOT call this tool if the basic parameters (e.g. text, parse_mode, caption, file, type, disable_notification) are sufficient for your task. " +
        `Accepts Telegram method names (e.g. "sendMessage", "sendPhoto") or Telesend types (e.g. "text", "photo", "poll").`,
      inputSchema: z.object({
        method: z
          .string()
          .min(1)
          .describe(
            `Telegram method name or Telesend type. Known values: ${KNOWN_QUERIES.join(", ")}`,
          ),
      }),
      annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true },
    },
    ({ method }) => {
      const doc = getMethodDoc(method);
      if (!doc) {
        return toolError(
          new AppError(
            "not_found",
            `No docs found for "${method}". Valid queries: ${KNOWN_QUERIES.join(", ")}`,
          ),
        );
      }
      return jsonResult(doc as unknown as Record<string, unknown>);
    },
  );

  // ── send_text ───────────────────────────────────────────────────────────────

  server.registerTool(
    "send_text",
    {
      title: "Send text message",
      description:
        'Send a Telegram text message. Use parse_mode: "HTML" or "MarkdownV2" for formatting. ' +
        "Add draft_id to send as a streaming draft instead of a regular message.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        text: z
          .string()
          .min(1)
          .optional()
          .describe("Message text. Required unless draft_id is set."),
        draft_id: z
          .number()
          .int()
          .positive()
          .optional()
          .describe(
            "Set to send as a streaming draft (thinking indicator) instead of a chat message.",
          ),
        parse_mode: z
          .enum(["HTML", "MarkdownV2", "Markdown"])
          .optional()
          .describe('Text formatting mode. Recommended: "HTML" or "MarkdownV2".'),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const { draft_id, text, ...rest } = payload;
        const isDraft = draft_id !== undefined;
        if (!isDraft && (text === undefined || text === "")) {
          throw new AppError("validation", 'Field "text" is required when draft_id is not set');
        }
        const type = isDraft ? "message-draft" : "text";
        const safePayload = (await policy.authorizePayload({
          ...rest,
          ...(isDraft ? { draft_id } : { text }),
        })) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_rich_message ───────────────────────────────────────────────────────

  server.registerTool(
    "send_rich_message",
    {
      title: "Send rich message",
      description:
        "Send a rich/structured Telegram message. " +
        "Add draft_id to send as a streaming draft instead.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        rich_message: z
          .object({})
          .loose()
          .describe("Rich message content, e.g. { markdown: '**Hello**' }."),
        draft_id: z
          .number()
          .int()
          .positive()
          .optional()
          .describe("Set to send as a streaming rich-message draft."),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const { draft_id, ...rest } = payload;
        const type = draft_id !== undefined ? "rich-message-draft" : "rich-message";
        const safePayload = (await policy.authorizePayload({
          ...rest,
          ...(draft_id !== undefined ? { draft_id } : {}),
        })) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_media ──────────────────────────────────────────────────────────────

  const mediaTypeEnum = Object.keys(MEDIA_TYPE_TO_CATALOG) as [string, ...string[]];

  server.registerTool(
    "send_media",
    {
      title: "Send media file",
      description:
        "Send a single media file. Set type to: photo, video, animation, audio, document, sticker, voice, video_note, or live_photo. " +
        "video_note and live_photo do not support URL sources. " +
        "live_photo also requires a photo field (the still frame). " +
        "Add caption for a text caption below the media.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        type: z.enum(mediaTypeEnum as [string, ...string[]]).describe("Media type to send."),
        file: mcpMediaSource(true, policy.includePaths).describe(
          "The media file. For video_note and live_photo, only path and file_id are accepted.",
        ),
        photo: mcpMediaSource(false, policy.includePaths)
          .optional()
          .describe(
            "Required only for live_photo: the still-frame companion photo (path or file_id only).",
          ),
        caption: z
          .string()
          .optional()
          .describe("Optional caption displayed below the media (max 1024 chars)."),
        parse_mode: z
          .enum(["HTML", "MarkdownV2", "Markdown"])
          .optional()
          .describe("Parse mode for the caption."),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const { type, file, ...rest } = payload;
        if (!type || !(type in MEDIA_TYPE_TO_CATALOG)) {
          throw new AppError(
            "validation",
            `Invalid media type "${String(type)}". Use: ${mediaTypeEnum.join(", ")}`,
          );
        }
        if (!file) throw new AppError("validation", 'Field "file" is required for send_media');
        const catalogType = MEDIA_TYPE_TO_CATALOG[type as string] as string;
        const operation = getOperation(catalogType);
        if (
          MEDIA_TYPES_NO_URL.has(type as string) &&
          (file as { source?: string }).source === "url"
        ) {
          throw new AppError(
            "validation",
            `URL source is not supported for type "${String(type)}"`,
          );
        }
        const rawPayload = remapMediaPayload(type as string, { ...rest, file });
        const safePayload = (await policy.authorizePayload(rawPayload)) as Record<string, unknown>;
        // Validate required fields via operation metadata
        for (const field of operation.requiredFields) {
          if (
            safePayload[field] === undefined ||
            safePayload[field] === null ||
            safePayload[field] === ""
          ) {
            throw new AppError(
              "validation",
              `Field "${field}" is required for type "${String(type)}"`,
            );
          }
        }
        return jsonResult(
          (await app.delivery.send({
            type: catalogType,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_media_group ────────────────────────────────────────────────────────

  server.registerTool(
    "send_media_group",
    {
      title: "Send media album",
      description:
        "Send 2–10 media files as an album. " +
        "Add star_count to send as paid media locked behind Telegram Stars. " +
        "Paid media does not support URL sources.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        media: z
          .array(
            z
              .object({
                type: z.string().min(1).describe("Item type: photo, video, audio, or document."),
                media: mcpMediaSource(true, policy.includePaths),
              })
              .loose(),
          )
          .describe("Array of 2–10 media items."),
        star_count: z
          .number()
          .int()
          .positive()
          .optional()
          .describe(
            "Set to send as paid media (Telegram Stars required to view). Disables URL sources.",
          ),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const { star_count, ...rest } = payload;
        const type = star_count !== undefined ? "paid-media" : "media-group";
        const safePayload = (await policy.authorizePayload({
          ...rest,
          ...(star_count !== undefined ? { star_count } : {}),
        })) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_location ───────────────────────────────────────────────────────────

  server.registerTool(
    "send_location",
    {
      title: "Send location or venue",
      description:
        "Send a geographic location. Add both title and address to send as a named venue instead.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        latitude: z.number().describe("Latitude of the location."),
        longitude: z.number().describe("Longitude of the location."),
        title: z
          .string()
          .min(1)
          .optional()
          .describe("Venue name. Required together with address to send a venue."),
        address: z
          .string()
          .min(1)
          .optional()
          .describe("Venue address. Required together with title to send a venue."),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const isVenue = payload.title !== undefined && payload.address !== undefined;
        if ((payload.title !== undefined) !== (payload.address !== undefined)) {
          throw new AppError("validation", "title and address must both be set to send a venue");
        }
        const type = isVenue ? "venue" : "location";
        const safePayload = (await policy.authorizePayload(payload)) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_contact ────────────────────────────────────────────────────────────

  server.registerTool(
    "send_contact",
    {
      title: "Send contact",
      description: "Send a phone contact card.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        phone_number: z.string().min(1).describe("Contact's phone number."),
        first_name: z.string().min(1).describe("Contact's first name."),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const safePayload = (await policy.authorizePayload(payload)) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type: "contact",
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_interactive ────────────────────────────────────────────────────────

  server.registerTool(
    "send_interactive",
    {
      title: "Send interactive content",
      description:
        "Send interactive content. Set type to: poll, checklist, dice, or game. " +
        "poll: requires question and options[]. " +
        "checklist: requires business_connection_id and checklist object. " +
        "dice: no required fields (optionally set emoji: 🎲🎯🏀⚽🎳🎰). " +
        "game: requires game_short_name.",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        type: z.enum(["poll", "checklist", "dice", "game"]).describe("Interactive content type."),
        // poll fields
        question: z.string().min(1).optional().describe("Poll question (required for type: poll)."),
        options: z
          .array(z.object({ text: z.string().min(1) }).loose())
          .optional()
          .describe("Poll answer options, 2–10 items (required for type: poll)."),
        // checklist fields
        business_connection_id: z
          .string()
          .min(1)
          .optional()
          .describe("Business connection ID (required for type: checklist)."),
        checklist: z
          .object({})
          .loose()
          .optional()
          .describe(
            "Checklist object (required for type: checklist). Shape: { title, tasks: [{id, text}], others_can_add_tasks?, others_can_mark_tasks_as_done? }.",
          ),
        // game fields
        game_short_name: z
          .string()
          .min(1)
          .optional()
          .describe("Short name of the HTML5 game from @BotFather (required for type: game)."),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const { type, ...rest } = payload;
        if (!type)
          throw new AppError("validation", 'Field "type" is required for send_interactive');
        // Validate required fields per type
        if (type === "poll") {
          if (!rest.question)
            throw new AppError("validation", 'Field "question" is required for type: poll');
          if (!rest.options)
            throw new AppError("validation", 'Field "options" is required for type: poll');
        } else if (type === "checklist") {
          if (!rest.business_connection_id)
            throw new AppError(
              "validation",
              'Field "business_connection_id" is required for type: checklist',
            );
          if (!rest.checklist)
            throw new AppError("validation", 'Field "checklist" is required for type: checklist');
        } else if (type === "game") {
          if (!rest.game_short_name)
            throw new AppError("validation", 'Field "game_short_name" is required for type: game');
        }
        const safePayload = (await policy.authorizePayload(rest)) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type: type as string,
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  // ── send_invoice ────────────────────────────────────────────────────────────

  server.registerTool(
    "send_invoice",
    {
      title: "Send invoice",
      description:
        "Send a payment invoice. Use currency: XTR for Telegram Stars payments (no provider_token needed).",
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: true,
      },
      inputSchema: envelopeSchema({
        title: z.string().min(1).describe("Product name (1–32 chars)."),
        description: z.string().min(1).describe("Product description (1–255 chars)."),
        payload: z
          .string()
          .min(1)
          .describe("Internal bot payload for this invoice (1–128 bytes, not shown to users)."),
        currency: z
          .string()
          .min(1)
          .describe('Three-letter ISO 4217 currency code, or "XTR" for Telegram Stars.'),
        prices: z
          .array(
            z
              .object({
                label: z.string().min(1),
                amount: z.number().int(),
              })
              .loose(),
          )
          .describe(
            "Price breakdown. Each item: { label, amount } where amount is in smallest currency units.",
          ),
      }),
    },
    async ({ to, bot, messageThreadId, payload }) => {
      try {
        if (!policy.canSend) throw new AppError("unauthorized", "MCP scope mcp:send is required");
        const safePayload = (await policy.authorizePayload(payload)) as Record<string, unknown>;
        return jsonResult(
          (await app.delivery.send({
            type: "invoice",
            to,
            ...(bot ? { bot } : {}),
            ...(messageThreadId ? { messageThreadId } : {}),
            payload: safePayload,
          })) as unknown as Record<string, unknown>,
        );
      } catch (error) {
        return toolError(error);
      }
    },
  );

  return server;
}

// ─── MCP server startup ───────────────────────────────────────────────────────

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

// ─── Payload authorization ────────────────────────────────────────────────────

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
      throw new AppError(
        "validation",
        "Remote MCP media must use a public URL or Telegram file_id; chat attachments and generated files cannot be uploaded directly",
      );
    }
    if (typeof record.value !== "string" || record.value.length === 0) {
      throw new AppError("validation", `MCP media source ${record.source} requires a value`);
    }
    if (record.source === "path") {
      throw new AppError(
        "filesystem_policy",
        "Remote MCP cannot use server filesystem paths; use a public HTTPS URL or Telegram file_id",
      );
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

// ─── Utilities ────────────────────────────────────────────────────────────────

/** @deprecated Use consolidated tool names. Kept for backward compatibility in tests. */
export function toolName(type: string): string {
  return `send_${type.replaceAll("-", "_")}`;
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
