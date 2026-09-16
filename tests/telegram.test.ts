import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { AliasRepository } from "../src/db/alias-repository.ts";
import { BotRepository } from "../src/db/bot-repository.ts";
import { openDatabase } from "../src/db/database.ts";
import { BotService } from "../src/services/bot-service.ts";
import { DeliveryService } from "../src/services/delivery-service.ts";
import { RecipientService } from "../src/services/recipient-service.ts";
import { MESSAGE_CATALOG } from "../src/telegram/catalog.ts";
import { type Fetch, TelegramClient } from "../src/telegram/client.ts";

const databases: Database[] = [];
const roots: string[] = [];
afterEach(async () => {
  for (const database of databases.splice(0)) database.close(false);
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function setup(fetcher: Fetch) {
  const database = await openDatabase(":memory:");
  databases.push(database);
  const bots = new BotRepository(database);
  const aliases = new AliasRepository(database);
  const telegram = new TelegramClient(fetcher);
  const botService = new BotService(bots, telegram);
  const recipients = new RecipientService(aliases);
  const delivery = new DeliveryService(botService, recipients, telegram);
  return { aliases, botService, bots, delivery, recipients, telegram };
}

describe("TelegramClient", () => {
  test("sends JSON and parses success", async () => {
    const calls: Array<{ url: string; init: RequestInit | undefined }> = [];
    const client = new TelegramClient((async (url, init) => {
      calls.push({ url: String(url), init });
      return Response.json({ ok: true, result: { message_id: 1 } });
    }) as Fetch);
    expect(
      await client.call<{ message_id: number }>("token", "sendMessage", {
        chat_id: "1",
        text: "hello",
      }),
    ).toEqual({ message_id: 1 });
    expect(calls[0]?.init?.headers).toEqual({ "content-type": "application/json" });
  });

  test("uploads local and nested media using multipart", async () => {
    let body: FormData | undefined;
    const client = new TelegramClient((async (_url, init) => {
      body = init?.body as FormData;
      return Response.json({ ok: true, result: [{ message_id: 2 }] });
    }) as Fetch);
    await client.call("token", "sendMediaGroup", {
      chat_id: "1",
      media: [
        {
          type: "photo",
          media: { source: "upload", file: new Blob(["image"]), filename: "x.jpg" },
        },
      ],
    });
    expect(body).toBeInstanceOf(FormData);
    expect(String(body?.get("media"))).toContain("attach://attachment_0");
    expect(body?.get("attachment_0")).toBeInstanceOf(Blob);
  });

  test("returns safe Telegram errors", async () => {
    const token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
    const client = new TelegramClient((async () =>
      Response.json(
        { ok: false, description: `bad ${token}`, error_code: 400 },
        { status: 400 },
      )) as Fetch);
    await expect(client.call(token, "sendMessage", {})).rejects.not.toThrow(token);
    await expect(client.call(token, "sendMessage", {})).rejects.toThrow(
      "Telegram sendMessage failed",
    );
  });
});

describe("bot and recipient services", () => {
  test("registers Telegram identity and resolves aliases with override", async () => {
    const app = await setup((async (url) => {
      if (String(url).endsWith("/getMe")) {
        return Response.json({
          ok: true,
          result: {
            id: 7,
            is_bot: true,
            first_name: "Alert",
            last_name: "Bot",
            username: "alert_bot",
          },
        });
      }
      return Response.json({ ok: true, result: { message_id: 3 } });
    }) as Fetch);
    expect(await app.botService.register("token")).toEqual({
      telegramId: "7",
      name: "Alert Bot",
      username: "alert_bot",
      isDefault: true,
    });
    app.aliases.create({ name: "ops", chatId: "-100", messageThreadId: 42 });
    expect(app.recipients.resolve("ops")).toEqual({ chatId: "-100", messageThreadId: 42 });
    expect(app.recipients.resolve("ops", 99)).toEqual({ chatId: "-100", messageThreadId: 99 });
    expect(app.recipients.resolve("123")).toEqual({ chatId: "123" });
  });

  test("does not store invalid bot credentials", async () => {
    const app = await setup((async () =>
      Response.json({ ok: false, description: "Unauthorized" }, { status: 401 })) as Fetch);
    await expect(app.botService.register("invalid")).rejects.toThrow("Unauthorized");
    expect(app.bots.list()).toHaveLength(0);
  });
});

describe("message catalog and delivery", () => {
  test("catalog matches the audited Bot API 10.3 content operations", () => {
    expect(MESSAGE_CATALOG.map(({ type }) => type)).toEqual([
      "text",
      "message-draft",
      "rich-message",
      "rich-message-draft",
      "animation",
      "audio",
      "document",
      "live-photo",
      "photo",
      "sticker",
      "video",
      "video-note",
      "voice",
      "paid-media",
      "media-group",
      "location",
      "venue",
      "contact",
      "poll",
      "checklist",
      "dice",
      "invoice",
      "game",
    ]);
    expect(
      MESSAGE_CATALOG.some(({ telegramMethod }) => telegramMethod === "callTelegram"),
    ).toBeFalse();
  });

  test("delivers every catalog entry through shared resolution", async () => {
    const methods: string[] = [];
    const app = await setup((async (url) => {
      const method = String(url).split("/").at(-1) as string;
      methods.push(method);
      return Response.json({ ok: true, result: { message_id: methods.length } });
    }) as Fetch);
    app.bots.add({ telegramId: "1", name: "Bot", username: "bot", token: "token" });
    app.aliases.create({ name: "ops", chatId: "-100", messageThreadId: 42 });
    for (const operation of MESSAGE_CATALOG) {
      const result = await app.delivery.send({
        type: operation.type,
        to: "ops",
        payload: operation.examplePayload,
      });
      expect(result.bot).toBe("bot");
      expect(result.messageThreadId).toBe(42);
    }
    expect(methods).toEqual(MESSAGE_CATALOG.map(({ telegramMethod }) => telegramMethod));
  });

  test("validates required fields and media source compatibility", async () => {
    const app = await setup((async () => Response.json({ ok: true, result: true })) as Fetch);
    app.bots.add({ telegramId: "1", name: "Bot", username: "bot", token: "token" });
    await expect(app.delivery.send({ type: "text", to: "1", payload: {} })).rejects.toThrow(
      'Field "text"',
    );
    await expect(
      app.delivery.send({
        type: "live-photo",
        to: "1",
        payload: {
          live_photo: { source: "url", value: "https://example.com/live.mp4" },
          photo: { source: "url", value: "https://example.com/photo.jpg" },
        },
      }),
    ).rejects.toThrow("URL media is not supported");
  });

  test("uploads every media catalog operation and nested collection", async () => {
    const contentTypes: string[] = [];
    const app = await setup((async (_url, init) => {
      contentTypes.push(init?.body instanceof FormData ? "multipart" : "json");
      return Response.json({ ok: true, result: { message_id: 1 } });
    }) as Fetch);
    app.bots.add({ telegramId: "1", name: "Bot", username: "bot", token: "token" });

    const mediaOperations = MESSAGE_CATALOG.filter(({ mediaFields }) => mediaFields.length > 0);
    for (const operation of mediaOperations) {
      await app.delivery.send({
        type: operation.type,
        to: "1",
        payload: replaceMedia(operation.examplePayload, {
          source: "upload",
          file: new Blob(["media"]),
          filename: "media.bin",
        }),
      });
    }
    expect(contentTypes).toEqual(mediaOperations.map(() => "multipart"));
  });

  test("uploads a local path without base64 conversion", async () => {
    const root = await mkdtemp(join(tmpdir(), "telesend-media-"));
    roots.push(root);
    const path = join(root, "report.txt");
    await Bun.write(path, "report");
    const uploaded: { value: File | null } = { value: null };
    const app = await setup((async (_url, init) => {
      const body = init?.body;
      if (!(body instanceof FormData)) throw new Error("Expected multipart body");
      uploaded.value = body.get("attachment_0") as File;
      return Response.json({ ok: true, result: { message_id: 1 } });
    }) as Fetch);
    app.bots.add({ telegramId: "1", name: "Bot", username: "bot", token: "token" });
    await app.delivery.send({
      type: "document",
      to: "1",
      payload: { document: { source: "path", value: path } },
    });
    expect(uploaded.value?.name).toBe("report.txt");
    expect(await uploaded.value?.text()).toBe("report");
  });
});

function replaceMedia(value: unknown, source: Record<string, unknown>): Record<string, unknown> {
  return replace(value, source) as Record<string, unknown>;
}

function replace(value: unknown, source: Record<string, unknown>): unknown {
  if (value && typeof value === "object" && "source" in value) return { ...source };
  if (Array.isArray(value)) return value.map((item) => replace(item, source));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, replace(item, source)]),
    );
  }
  return value;
}
