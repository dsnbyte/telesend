import { basename } from "node:path";
import { AppError, redactSecrets } from "../core/errors.ts";
import type { MediaSource } from "./catalog.ts";

interface TelegramEnvelope<T> {
  ok: boolean;
  result?: T;
  description?: string;
  error_code?: number;
}

export interface TelegramUser {
  id: number;
  is_bot: boolean;
  first_name: string;
  last_name?: string;
  username?: string;
}

export type Fetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export class TelegramClient {
  constructor(
    private readonly fetcher: Fetch = fetch as Fetch,
    private readonly baseUrl = "https://api.telegram.org",
  ) {}

  getMe(token: string): Promise<TelegramUser> {
    return this.call<TelegramUser>(token, "getMe", {});
  }

  async call<T>(token: string, method: string, payload: Record<string, unknown>): Promise<T> {
    const materialized = await materializePayload(payload);
    const url = `${this.baseUrl}/bot${token}/${method}`;
    const init: RequestInit = { method: "POST" };
    if (materialized.files.length > 0) {
      const body = new FormData();
      for (const [key, value] of Object.entries(materialized.payload)) {
        if (value === undefined || value === null) continue;
        body.set(key, typeof value === "object" ? JSON.stringify(value) : String(value));
      }
      for (const file of materialized.files) {
        body.set(file.name, new File([file.blob], file.filename, { type: file.blob.type }));
      }
      init.body = body;
    } else {
      init.headers = { "content-type": "application/json" };
      init.body = JSON.stringify(materialized.payload);
    }

    let response: Response;
    try {
      response = await this.fetcher(url, init);
    } catch (error) {
      throw new AppError("telegram", `Telegram request failed: ${redactSecrets(error, [token])}`);
    }

    let envelope: TelegramEnvelope<T>;
    try {
      envelope = (await response.json()) as TelegramEnvelope<T>;
    } catch {
      throw new AppError("telegram", `Telegram returned an invalid response for ${method}`);
    }
    if (!response.ok || !envelope.ok || envelope.result === undefined) {
      const description = redactSecrets(envelope.description ?? `HTTP ${response.status}`, [token]);
      throw new AppError("telegram", `Telegram ${method} failed: ${description}`, {
        telegramErrorCode: envelope.error_code,
      });
    }
    return envelope.result;
  }
}

interface Upload {
  name: string;
  blob: Blob;
  filename: string;
}

async function materializePayload(payload: Record<string, unknown>): Promise<{
  payload: Record<string, unknown>;
  files: Upload[];
}> {
  const files: Upload[] = [];
  const mapped = (await materializeValue(payload, files, "file")) as Record<string, unknown>;
  return { payload: mapped, files };
}

async function materializeValue(value: unknown, files: Upload[], hint: string): Promise<unknown> {
  if (isMediaSource(value)) {
    if (value.source === "file_id" || value.source === "url") return requireMediaValue(value);
    const blob = value.source === "path" ? Bun.file(requireMediaValue(value)) : value.file;
    if (!blob) throw new AppError("validation", "Uploaded media is missing file content");
    const name = `attachment_${files.length}`;
    const filename =
      value.filename ?? (value.source === "path" ? basename(requireMediaValue(value)) : hint);
    files.push({ name, blob, filename });
    return `attach://${name}`;
  }
  if (Array.isArray(value)) {
    return Promise.all(
      value.map((item, index) => materializeValue(item, files, `${hint}_${index}`)),
    );
  }
  if (value && typeof value === "object" && !(value instanceof Blob)) {
    const entries = await Promise.all(
      Object.entries(value).map(async ([key, item]) => [
        key,
        await materializeValue(item, files, key),
      ]),
    );
    return Object.fromEntries(entries);
  }
  return value;
}

function isMediaSource(value: unknown): value is MediaSource {
  if (!value || typeof value !== "object") return false;
  const source = (value as { source?: unknown }).source;
  return source === "file_id" || source === "url" || source === "path" || source === "upload";
}

function requireMediaValue(source: MediaSource): string {
  if (!source.value)
    throw new AppError("validation", `Media source ${source.source} requires a value`);
  return source.value;
}
