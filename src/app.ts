import type { Database } from "bun:sqlite";
import { AliasRepository } from "./db/alias-repository.ts";
import { BotRepository } from "./db/bot-repository.ts";
import { openDatabase } from "./db/database.ts";
import { BotService } from "./services/bot-service.ts";
import { DeliveryService } from "./services/delivery-service.ts";
import { RecipientService } from "./services/recipient-service.ts";
import { type Fetch, TelegramClient } from "./telegram/client.ts";

export interface Application {
  aliases: AliasRepository;
  bots: BotService;
  database: Database;
  databasePath: string;
  delivery: DeliveryService;
}

export async function createApplication(options: {
  databasePath: string;
  fetcher?: Fetch;
  telegramBaseUrl?: string;
}): Promise<Application> {
  const database = await openDatabase(options.databasePath);
  const aliases = new AliasRepository(database);
  const botRepository = new BotRepository(database);
  const telegram = new TelegramClient(options.fetcher, options.telegramBaseUrl);
  const bots = new BotService(botRepository, telegram);
  const recipients = new RecipientService(aliases);
  const delivery = new DeliveryService(bots, recipients, telegram);
  return { aliases, bots, database, databasePath: options.databasePath, delivery };
}
