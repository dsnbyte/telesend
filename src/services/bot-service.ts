import { AppError } from "../core/errors.ts";
import type { SafeBot } from "../core/result.ts";
import type { BotRepository, StoredBot } from "../db/bot-repository.ts";
import type { TelegramClient } from "../telegram/client.ts";

export class BotService {
  constructor(
    private readonly bots: BotRepository,
    private readonly telegram: TelegramClient,
  ) {}

  async register(token: string): Promise<SafeBot> {
    if (!token.trim()) throw new AppError("validation", "Bot token is required");
    const user = await this.telegram.getMe(token.trim());
    if (!user.is_bot || !user.username) {
      throw new AppError("validation", "Telegram token did not resolve to a bot with a username");
    }
    return this.bots.add({
      telegramId: String(user.id),
      name: [user.first_name, user.last_name].filter(Boolean).join(" "),
      username: user.username,
      token: token.trim(),
    });
  }

  list(): SafeBot[] {
    return this.bots.list();
  }

  setDefault(username: string): SafeBot {
    return this.bots.setDefault(username);
  }

  remove(username: string): SafeBot {
    return this.bots.remove(username);
  }

  resolve(username?: string): StoredBot {
    const bot = username ? this.bots.find(username) : this.bots.getDefault();
    if (bot) return bot;
    if (username)
      throw new AppError("not_found", `Bot @${username.replace(/^@/, "")} was not found`);
    throw new AppError(
      "validation",
      "No default bot is configured; register or select a bot first",
    );
  }
}
