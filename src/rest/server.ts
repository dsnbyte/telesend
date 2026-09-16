import { timingSafeEqual } from "node:crypto";
import type { Application } from "../app.ts";
import { AppError, redactSecrets } from "../core/errors.ts";
import { getOperation } from "../telegram/catalog.ts";

export interface RestOptions {
  hostname?: string;
  port?: number;
}

export interface RestServer {
  readonly url: URL;
  stop(closeActiveConnections?: boolean): void | Promise<void>;
}

interface RestRuntime {
  apiKey?: string;
  log?: (message: string) => void;
  serve?: (options: {
    hostname?: string;
    port?: number;
    fetch(request: Request): Response | Promise<Response>;
  }) => RestServer;
}

const MAX_REQUEST_BYTES = 52 * 1024 * 1024;

export function startRestServer(
  app: Application,
  options: RestOptions,
  runtime: RestRuntime = {},
): RestServer {
  const apiKey = (runtime.apiKey ?? process.env.TELESEND_API_KEY)?.trim();
  if (!apiKey) {
    throw new AppError("configuration", "TELESEND_API_KEY is required to start the REST server");
  }
  const handler = createRestHandler(app, apiKey);
  const server = (runtime.serve ?? ((value) => Bun.serve(value)))({
    ...(options.hostname ? { hostname: options.hostname } : {}),
    ...(options.port === undefined ? {} : { port: options.port }),
    fetch: handler,
  });
  (runtime.log ?? console.log)(`Telesend REST API listening on ${server.url.toString()}`);
  return server;
}

export function createRestHandler(app: Application, apiKey: string) {
  if (!apiKey.trim()) throw new AppError("configuration", "REST API key cannot be empty");
  return async (request: Request): Promise<Response> => {
    try {
      if (!authenticated(request.headers.get("x-api-key"), apiKey)) {
        throw new AppError("unauthorized", "Invalid API key");
      }
      const length = Number(request.headers.get("content-length") ?? 0);
      if (Number.isFinite(length) && length > MAX_REQUEST_BYTES) {
        return json({ error: { code: "validation", message: "Request body is too large" } }, 413);
      }
      return await route(app, request);
    } catch (error) {
      return errorResponse(error, [apiKey]);
    }
  };
}

async function route(app: Application, request: Request): Promise<Response> {
  const url = new URL(request.url);
  const path = url.pathname;
  const method = request.method.toUpperCase();

  if (method === "GET" && path === "/health") return json({ status: "ok" });
  if (method === "GET" && path === "/bots") return json({ data: app.bots.list() });
  if (method === "POST" && path === "/bots") {
    const body = await readJson(request);
    return json({ data: await app.bots.register(stringField(body, "token")) }, 201);
  }

  const botMatch = path.match(/^\/bots\/([^/]+)(\/default)?$/);
  if (botMatch) {
    const username = decodeURIComponent(botMatch[1] as string);
    if (method === "POST" && botMatch[2] === "/default") {
      return json({ data: app.bots.setDefault(username) });
    }
    if (method === "DELETE" && !botMatch[2]) return json({ data: app.bots.remove(username) });
  }

  if (method === "GET" && path === "/aliases") return json({ data: app.aliases.list() });
  if (method === "POST" && path === "/aliases") {
    const body = await readJson(request);
    return json({ data: app.aliases.create(aliasInput(body)) }, 201);
  }

  const aliasMatch = path.match(/^\/aliases\/([^/]+)$/);
  if (aliasMatch) {
    const name = decodeURIComponent(aliasMatch[1] as string);
    if (method === "DELETE") return json({ data: app.aliases.remove(name) });
    if (method === "PATCH") {
      const existing = app.aliases.find(name);
      if (!existing) throw new AppError("not_found", `Alias "${name}" was not found`);
      const body = await readJson(request);
      return json({
        data: app.aliases.update(name, {
          name: optionalString(body, "name") ?? existing.name,
          chatId: optionalString(body, "chatId") ?? existing.chatId,
          messageThreadId: optionalNullableInteger(
            body,
            "messageThreadId",
            existing.messageThreadId,
          ),
        }),
      });
    }
  }

  const messageMatch = path.match(/^\/messages\/([^/]+)$/);
  if (method === "POST" && messageMatch) {
    const type = decodeURIComponent(messageMatch[1] as string);
    try {
      getOperation(type);
    } catch {
      throw new AppError("not_found", `Message type "${type}" was not found`);
    }
    const input = await readMessageRequest(request);
    rejectServerPaths(input.payload);
    return json({
      data: await app.delivery.send({
        type,
        to: input.to,
        payload: input.payload,
        ...(input.bot ? { bot: input.bot } : {}),
        ...(input.messageThreadId === undefined ? {} : { messageThreadId: input.messageThreadId }),
      }),
    });
  }

  throw new AppError("not_found", "Endpoint not found");
}

async function readMessageRequest(request: Request): Promise<{
  to: string;
  bot?: string;
  messageThreadId?: number;
  payload: Record<string, unknown>;
}> {
  const contentType = request.headers.get("content-type") ?? "";
  if (!contentType.includes("multipart/form-data")) {
    const body = await readJson(request);
    const payloadValue = body.payload;
    if (!payloadValue || typeof payloadValue !== "object" || Array.isArray(payloadValue)) {
      throw new AppError("validation", 'Field "payload" must be an object');
    }
    const bot = optionalString(body, "bot");
    return {
      to: stringField(body, "to"),
      payload: payloadValue as Record<string, unknown>,
      ...(bot ? { bot } : {}),
      ...(body.messageThreadId === undefined
        ? {}
        : { messageThreadId: integerField(body, "messageThreadId") }),
    };
  }

  const form = await request.formData();
  const to = form.get("to");
  if (typeof to !== "string" || !to.trim())
    throw new AppError("validation", 'Field "to" is required');
  const rawPayload = form.get("payload");
  let payload: Record<string, unknown> = {};
  if (typeof rawPayload === "string" && rawPayload) {
    try {
      const parsed = JSON.parse(rawPayload);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error();
      payload = parsed as Record<string, unknown>;
    } catch {
      throw new AppError("validation", 'Multipart field "payload" must be a JSON object');
    }
  }
  payload = resolveUploadReferences(payload, form) as Record<string, unknown>;
  for (const [key, value] of form.entries()) {
    if (key === "to" || key === "bot" || key === "messageThreadId" || key === "payload") continue;
    if (typeof value !== "string" && payload[key] === undefined) {
      const file = value as File;
      payload[key] = { source: "upload", file, filename: file.name };
    }
  }
  const bot = form.get("bot");
  const thread = form.get("messageThreadId");
  return {
    to: to.trim(),
    payload,
    ...(typeof bot === "string" && bot.trim() ? { bot: bot.trim() } : {}),
    ...(typeof thread === "string" && thread
      ? { messageThreadId: parseInteger(thread, "messageThreadId") }
      : {}),
  };
}

function resolveUploadReferences(value: unknown, form: FormData): unknown {
  if (Array.isArray(value)) return value.map((item) => resolveUploadReferences(item, form));
  if (!value || typeof value !== "object") return value;
  const record = value as Record<string, unknown>;
  if (record.source === "upload" && typeof record.value === "string") {
    const file = form.get(record.value);
    if (!(file instanceof File)) {
      throw new AppError("validation", `Multipart file "${record.value}" is required`);
    }
    return { source: "upload", file, filename: file.name };
  }
  return Object.fromEntries(
    Object.entries(record).map(([key, item]) => [key, resolveUploadReferences(item, form)]),
  );
}

function rejectServerPaths(value: unknown): void {
  if (Array.isArray(value)) {
    for (const item of value) rejectServerPaths(item);
    return;
  }
  if (!value || typeof value !== "object") return;
  const record = value as Record<string, unknown>;
  if (record.source === "path") {
    throw new AppError("validation", "REST requests cannot use server filesystem paths");
  }
  for (const item of Object.values(record)) rejectServerPaths(item);
}

function authenticated(provided: string | null, expected: string): boolean {
  if (provided === null) return false;
  const actualBytes = Buffer.from(provided);
  const expectedBytes = Buffer.from(expected);
  return actualBytes.length === expectedBytes.length && timingSafeEqual(actualBytes, expectedBytes);
}

function aliasInput(body: Record<string, unknown>) {
  return {
    name: stringField(body, "name"),
    chatId: stringField(body, "chatId"),
    ...(body.messageThreadId === undefined || body.messageThreadId === null
      ? {}
      : { messageThreadId: integerField(body, "messageThreadId") }),
  };
}

async function readJson(request: Request): Promise<Record<string, unknown>> {
  let parsed: unknown;
  try {
    parsed = await request.json();
  } catch {
    throw new AppError("validation", "Request body must be valid JSON");
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new AppError("validation", "Request body must be a JSON object");
  }
  return parsed as Record<string, unknown>;
}

function stringField(body: Record<string, unknown>, name: string): string {
  const value = body[name];
  if (typeof value !== "string" || !value.trim()) {
    throw new AppError("validation", `Field "${name}" is required`);
  }
  return value.trim();
}

function optionalString(body: Record<string, unknown>, name: string): string | undefined {
  const value = body[name];
  if (value === undefined) return undefined;
  if (typeof value !== "string" || !value.trim()) {
    throw new AppError("validation", `Field "${name}" must be a non-empty string`);
  }
  return value.trim();
}

function integerField(body: Record<string, unknown>, name: string): number {
  const value = body[name];
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value <= 0) {
    throw new AppError("validation", `Field "${name}" must be a positive integer`);
  }
  return value;
}

function optionalNullableInteger(
  body: Record<string, unknown>,
  name: string,
  fallback: number | null,
): number | null {
  const value = body[name];
  if (value === undefined) return fallback;
  if (value === null) return null;
  return integerField(body, name);
}

function parseInteger(value: string, name: string): number {
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    throw new AppError("validation", `Field "${name}" must be a positive integer`);
  }
  return parsed;
}

function errorResponse(error: unknown, secrets: readonly string[]): Response {
  const appError =
    error instanceof AppError ? error : new AppError("telegram", redactSecrets(error, secrets));
  const status = {
    unauthorized: 401,
    validation: 400,
    not_found: 404,
    conflict: 409,
    filesystem_policy: 403,
    configuration: 500,
    telegram: 502,
  }[appError.code];
  return json(
    {
      error: {
        code: appError.code,
        message: redactSecrets(appError.message, secrets),
        ...(appError.details ? { details: appError.details } : {}),
      },
    },
    status,
  );
}

function json(value: unknown, status = 200): Response {
  return Response.json(value, { status, headers: { "cache-control": "no-store" } });
}
