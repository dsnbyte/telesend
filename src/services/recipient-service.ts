import { AppError } from "../core/errors.ts";
import type { AliasRepository } from "../db/alias-repository.ts";

export interface ResolvedRecipient {
  chatId: string;
  messageThreadId?: number;
}

const CHAT_ID = /^-?\d+$/;

export class RecipientService {
  constructor(private readonly aliases: AliasRepository) {}

  resolve(target: string, explicitThreadId?: number): ResolvedRecipient {
    if (
      explicitThreadId !== undefined &&
      (!Number.isSafeInteger(explicitThreadId) || explicitThreadId <= 0)
    ) {
      throw new AppError("validation", "Message thread ID must be a positive integer");
    }
    const normalized = target.trim();
    if (CHAT_ID.test(normalized)) {
      return explicitThreadId === undefined
        ? { chatId: normalized }
        : { chatId: normalized, messageThreadId: explicitThreadId };
    }
    const alias = this.aliases.find(normalized);
    if (!alias) throw new AppError("not_found", `Recipient alias "${normalized}" was not found`);
    const thread = explicitThreadId ?? alias.messageThreadId ?? undefined;
    return thread === undefined
      ? { chatId: alias.chatId }
      : { chatId: alias.chatId, messageThreadId: thread };
  }
}
