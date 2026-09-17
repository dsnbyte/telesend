import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { createApplication } from "../src/app.ts";
import {
  createRestHandler,
  loadRestAuthConfig,
  type RestServer,
  startRestServer,
} from "../src/rest/server.ts";
import { MESSAGE_CATALOG } from "../src/telegram/catalog.ts";
import type { Fetch } from "../src/telegram/client.ts";

const API_KEY = "test-api-key";
const databases: Database[] = [];
const servers: RestServer[] = [];
afterEach(async () => {
  for (const server of servers.splice(0)) await server.stop(true);
  for (const database of databases.splice(0)) database.close(false);
});

async function setup() {
  const telegramCalls: string[] = [];
  const fetcher = (async (url) => {
    const method = String(url).split("/").at(-1) as string;
    telegramCalls.push(method);
    if (method === "getMe") {
      return Response.json({
        ok: true,
        result: {
          id: telegramCalls.length,
          is_bot: true,
          first_name: "REST",
          username: `rest_${telegramCalls.length}_bot`,
        },
      });
    }
    return Response.json({ ok: true, result: { message_id: telegramCalls.length } });
  }) as Fetch;
  const app = await createApplication({ databasePath: ":memory:", fetcher });
  databases.push(app.database);
  const handler = createRestHandler(app, await Bun.password.hash(API_KEY));
  const request = (path: string, init: RequestInit = {}) =>
    handler(
      new Request(`http://localhost${path}`, {
        ...init,
        headers: { "x-api-key": API_KEY, ...init.headers },
      }),
    );
  return { app, handler, request, telegramCalls };
}

describe("REST startup and authentication", () => {
  test("refuses to bind without an API key", async () => {
    const { app } = await setup();
    let binds = 0;
    expect(() =>
      startRestServer(
        app,
        {},
        {
          apiKey: "",
          serve: () => {
            binds += 1;
            return {} as RestServer;
          },
        },
      ),
    ).toThrow("TELESEND_API_KEY or TELESEND_API_KEY_HASH is required");
    expect(binds).toBe(0);
  });

  test("hashes a plaintext API key", async () => {
    const config = loadRestAuthConfig({ TELESEND_API_KEY: API_KEY });
    expect(await Bun.password.verify(API_KEY, config.apiKeyHash)).toBe(true);
  });

  test("prefers a configured API key hash over a plaintext API key", async () => {
    const configuredApiKey = "configured-api-key";
    const config = loadRestAuthConfig({
      TELESEND_API_KEY: API_KEY,
      TELESEND_API_KEY_HASH: await Bun.password.hash(configuredApiKey),
    });

    expect(await Bun.password.verify(configuredApiKey, config.apiKeyHash)).toBe(true);
    expect(await Bun.password.verify(API_KEY, config.apiKeyHash)).toBe(false);
  });

  test("does not fall back to a plaintext API key when the configured hash is invalid", () => {
    expect(() =>
      loadRestAuthConfig({
        TELESEND_API_KEY: API_KEY,
        TELESEND_API_KEY_HASH: "not-a-password-hash",
      }),
    ).toThrow("TELESEND_API_KEY_HASH must contain a Bun-compatible password hash");
  });

  test("reports the actual bound address", async () => {
    const { app } = await setup();
    const messages: string[] = [];
    const server = startRestServer(
      app,
      { hostname: "127.0.0.1", port: 0 },
      { apiKey: API_KEY, log: (value) => messages.push(value) },
    );
    servers.push(server);
    expect(server.url.port).not.toBe("0");
    expect(messages[0]).toContain(server.url.toString());
  });

  test("authenticates before routing or side effects", async () => {
    const { handler, telegramCalls } = await setup();
    const response = await handler(
      new Request("http://localhost/bots", {
        method: "POST",
        body: JSON.stringify({ token: "secret" }),
        headers: { "content-type": "application/json", "x-api-key": "wrong" },
      }),
    );
    expect(response.status).toBe(401);
    expect(telegramCalls).toHaveLength(0);
    expect(await response.text()).not.toContain(API_KEY);
  });

  test("authenticates requests with a precomputed API key hash", async () => {
    const { app } = await setup();
    const apiKeyHash = await Bun.password.hash(API_KEY);
    const handler = createRestHandler(app, apiKeyHash);

    expect(
      (await handler(new Request("http://localhost/health", { headers: { "x-api-key": API_KEY } })))
        .status,
    ).toBe(200);
    expect(
      (await handler(new Request("http://localhost/health", { headers: { "x-api-key": "wrong" } })))
        .status,
    ).toBe(401);
  });
});

describe("REST resources", () => {
  test("supports bot and alias lifecycle on unversioned routes", async () => {
    const api = await setup();
    let response = await api.request("/bots", {
      method: "POST",
      body: JSON.stringify({ token: "secret-token" }),
      headers: { "content-type": "application/json" },
    });
    expect(response.status).toBe(201);
    expect(await response.text()).not.toContain("secret-token");

    response = await api.request("/aliases", {
      method: "POST",
      body: JSON.stringify({ name: "ops", chatId: "-100", messageThreadId: 42 }),
      headers: { "content-type": "application/json" },
    });
    expect(response.status).toBe(201);
    response = await api.request("/aliases/ops", {
      method: "PATCH",
      body: JSON.stringify({ name: "releases", messageThreadId: 99 }),
      headers: { "content-type": "application/json" },
    });
    expect((await response.json()).data.messageThreadId).toBe(99);
    expect((await api.request("/health")).status).toBe(200);
    expect((await api.request("/v1/messages/text", { method: "POST" })).status).toBe(404);
  });

  test("maps stable safe errors", async () => {
    const api = await setup();
    const response = await api.request("/aliases", {
      method: "POST",
      body: "bad json",
      headers: { "content-type": "application/json" },
    });
    expect(response.status).toBe(400);
    expect(await response.json()).toEqual({
      error: { code: "validation", message: "Request body must be valid JSON" },
    });
  });
});

describe("REST messages", () => {
  test("exposes a JSON route for every catalog operation", async () => {
    const api = await setup();
    await api.app.bots.register("token");
    for (const operation of MESSAGE_CATALOG) {
      const response = await api.request(`/messages/${operation.type}`, {
        method: "POST",
        body: JSON.stringify({ to: "1", payload: operation.examplePayload }),
        headers: { "content-type": "application/json" },
      });
      expect(response.status).toBe(200);
    }
  }, 15_000);

  test("supports multipart uploads for every media operation", async () => {
    const api = await setup();
    await api.app.bots.register("token");
    for (const operation of MESSAGE_CATALOG.filter(({ mediaFields }) => mediaFields.length > 0)) {
      const form = new FormData();
      form.set("to", "1");
      form.set("payload", JSON.stringify(replaceMedia(operation.examplePayload)));
      form.set("file", new File(["content"], "media.bin"));
      const response = await api.request(`/messages/${operation.type}`, {
        method: "POST",
        body: form,
      });
      expect(response.status).toBe(200);
    }
  });

  test("rejects server paths before Telegram delivery", async () => {
    const api = await setup();
    await api.app.bots.register("token");
    const before = api.telegramCalls.length;
    const response = await api.request("/messages/document", {
      method: "POST",
      body: JSON.stringify({
        to: "1",
        payload: { document: { source: "path", value: "/etc/passwd" } },
      }),
      headers: { "content-type": "application/json" },
    });
    expect(response.status).toBe(400);
    expect(api.telegramCalls).toHaveLength(before);
  });
});

function replaceMedia(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(replaceMedia);
  if (value && typeof value === "object" && "source" in value) {
    return { source: "upload", value: "file" };
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, replaceMedia(item)]),
    );
  }
  return value;
}
