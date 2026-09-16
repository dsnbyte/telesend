import { AppError } from "../core/errors.ts";
import type { DeliveryResult } from "../core/result.ts";
import { getOperation, type MediaSource, type MessageOperation } from "../telegram/catalog.ts";
import type { TelegramClient } from "../telegram/client.ts";
import type { BotService } from "./bot-service.ts";
import type { RecipientService } from "./recipient-service.ts";

export interface DeliveryRequest {
  type: string;
  to: string;
  bot?: string;
  messageThreadId?: number;
  payload: Record<string, unknown>;
}

export class DeliveryService {
  constructor(
    private readonly bots: BotService,
    private readonly recipients: RecipientService,
    private readonly telegram: TelegramClient,
  ) {}

  async send(request: DeliveryRequest): Promise<DeliveryResult> {
    let operation: MessageOperation;
    try {
      operation = getOperation(request.type);
    } catch {
      throw new AppError("validation", `Unsupported message type: ${request.type}`);
    }
    for (const field of operation.requiredFields) {
      const value = request.payload[field];
      if (value === undefined || value === null || value === "") {
        throw new AppError("validation", `Field "${field}" is required for ${request.type}`);
      }
    }
    validateMediaSources(request.payload, operation.allowUrl);

    const bot = this.bots.resolve(request.bot);
    const recipient = this.recipients.resolve(request.to, request.messageThreadId);
    const payload: Record<string, unknown> = { ...request.payload, chat_id: recipient.chatId };
    if (recipient.messageThreadId !== undefined)
      payload.message_thread_id = recipient.messageThreadId;

    const result = await this.telegram.call<Record<string, unknown> | boolean | unknown[]>(
      bot.token,
      operation.telegramMethod,
      payload,
    );
    const messageId = extractMessageId(result);
    return {
      bot: bot.username,
      chatId: recipient.chatId,
      ...(recipient.messageThreadId === undefined
        ? {}
        : { messageThreadId: recipient.messageThreadId }),
      ...(messageId === undefined ? {} : { messageId }),
      result,
    };
  }
}

function extractMessageId(result: unknown): number | undefined {
  if (result && typeof result === "object" && !Array.isArray(result)) {
    const value = (result as { message_id?: unknown }).message_id;
    if (typeof value === "number") return value;
  }
  if (Array.isArray(result)) {
    const value = (result[0] as { message_id?: unknown } | undefined)?.message_id;
    if (typeof value === "number") return value;
  }
  return undefined;
}

function validateMediaSources(value: unknown, allowUrl: boolean, key = ""): void {
  if (isMediaSource(value)) {
    if (value.source === "url" && !allowUrl) {
      throw new AppError("validation", "URL media is not supported for this message type");
    }
    if (key === "thumbnail" && value.source !== "path" && value.source !== "upload") {
      throw new AppError("validation", "Thumbnail must be a new uploaded file");
    }
    if (
      (value.source === "file_id" || value.source === "url" || value.source === "path") &&
      !value.value
    ) {
      throw new AppError("validation", `Media source ${value.source} requires a value`);
    }
    if (value.source === "upload" && !value.file) {
      throw new AppError("validation", "Uploaded media is missing file content");
    }
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) validateMediaSources(item, allowUrl, key);
  } else if (value && typeof value === "object") {
    for (const [childKey, item] of Object.entries(value))
      validateMediaSources(item, allowUrl, childKey);
  }
}

function isMediaSource(value: unknown): value is MediaSource {
  if (!value || typeof value !== "object") return false;
  return ["file_id", "url", "path", "upload"].includes(
    String((value as { source?: unknown }).source),
  );
}
