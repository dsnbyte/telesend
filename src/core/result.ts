export interface DeliveryResult<T = unknown> {
  bot: string;
  chatId: string;
  messageThreadId?: number;
  messageId?: number;
  result: T;
}

export interface SafeBot {
  telegramId: string;
  name: string;
  username: string;
  isDefault: boolean;
}

export type AliasType = "group" | "private";

export interface RecipientAlias {
  name: string;
  chatId: string;
  messageThreadId: number | null;
  type: AliasType;
}
