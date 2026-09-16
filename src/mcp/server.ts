import { McpServer } from "@modelcontextprotocol/server";
import { StdioServerTransport } from "@modelcontextprotocol/server/stdio";
import { z } from "zod";
import type { Application } from "../app.ts";
import { AppError, toAppError } from "../core/errors.ts";
import { MESSAGE_CATALOG } from "../telegram/catalog.ts";
import { loadMcpConfig } from "./config.ts";
import { FilePolicy } from "./file-policy.ts";

export interface McpOptions {
  allowPaths: string[];
  configPath?: string;
}

export function createMcpServer(app: Application, filePolicy: FilePolicy): McpServer {
  const server = new McpServer({ name: "telesend", version: "0.1.0" });

  for (const operation of MESSAGE_CATALOG) {
    const requiredPayload = Object.fromEntries(
      operation.requiredFields.map((field) => [field, z.unknown()]),
    );
    server.registerTool(
      toolName(operation.type),
      {
        title: `Send ${operation.description}`,
        description: `Send a Telegram ${operation.type} message`,
        inputSchema: z.object({
          to: z.string().min(1).describe("Chat ID, @username, or configured alias"),
          bot: z.string().min(1).optional().describe("Bot username; uses the default when omitted"),
          messageThreadId: z.number().int().positive().optional(),
          payload: z.object(requiredPayload).loose(),
        }),
      },
      async ({ to, bot, messageThreadId, payload }) => {
        try {
          const safePayload = (await authorizeMcpPayload(payload, filePolicy)) as Record<
            string,
            unknown
          >;
          const result = await app.delivery.send({
            type: operation.type,
            to,
            ...(bot === undefined ? {} : { bot }),
            ...(messageThreadId === undefined ? {} : { messageThreadId }),
            payload: safePayload,
          });
          return {
            content: [{ type: "text" as const, text: JSON.stringify(result) }],
            structuredContent: result as unknown as Record<string, unknown>,
          };
        } catch (error) {
          const safe = toAppError(error);
          return {
            isError: true,
            content: [{ type: "text" as const, text: `${safe.code}: ${safe.message}` }],
          };
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

export function toolName(type: string): string {
  return `send_${type.replaceAll("-", "_")}`;
}

function isMcpSource(value: unknown): value is "file_id" | "path" | "url" {
  return value === "file_id" || value === "path" || value === "url";
}
