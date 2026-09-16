import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PassThrough } from "node:stream";
import { LATEST_PROTOCOL_VERSION } from "@modelcontextprotocol/server";
import { StdioServerTransport } from "@modelcontextprotocol/server/stdio";
import { type Application, createApplication } from "../src/app.ts";
import { FilePolicy } from "../src/mcp/file-policy.ts";
import { authorizeMcpPayload, createMcpServer, toolName } from "../src/mcp/server.ts";
import { MESSAGE_CATALOG } from "../src/telegram/catalog.ts";
import type { Fetch } from "../src/telegram/client.ts";
import { TestMcpClient } from "./helpers/mcp-client.ts";

const applications: Application[] = [];
const roots: string[] = [];

afterEach(async () => {
  for (const app of applications.splice(0)) app.database.close(false);
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function setup(fetcher: Fetch) {
  const root = await mkdtemp(join(tmpdir(), "telesend-mcp-"));
  roots.push(root);
  const app = await createApplication({
    databasePath: ":memory:",
    fetcher: (async (url, init) => {
      if (String(url).endsWith("/getMe")) {
        const token = String(url).match(/\/bot([^/]+)\//)?.[1] ?? "bot";
        const suffix = token === "two" ? "other" : "default";
        return Response.json({
          ok: true,
          result: {
            id: token === "two" ? 2 : 1,
            is_bot: true,
            first_name: suffix,
            username: `${suffix}_bot`,
          },
        });
      }
      return fetcher(url, init);
    }) as Fetch,
  });
  applications.push(app);
  const policy = await FilePolicy.create({
    cwd: root,
    configuredRoots: [],
    cliRoots: [],
    include: [],
    exclude: [],
    protectedPaths: [],
  });
  const server = createMcpServer(app, policy);
  const client = await TestMcpClient.connect(server);
  return { app, client, policy, root, server };
}

describe("MCP interface", () => {
  test("lists only one content-specific tool per catalog entry", async () => {
    const { client } = await setup(okFetch);
    const result = await client.request("tools/list", {});
    const tools = (result as { tools: Array<{ name: string }> }).tools;
    expect(tools.map(({ name }) => name)).toEqual(
      MESSAGE_CATALOG.map(({ type }) => toolName(type)),
    );
    expect(tools.some(({ name }) => /bot|alias|telegram_method/.test(name))).toBeFalse();
  });

  test("writes only newline-delimited protocol frames to stdout", async () => {
    const root = await mkdtemp(join(tmpdir(), "telesend-mcp-stdio-"));
    roots.push(root);
    const app = await createApplication({ databasePath: ":memory:", fetcher: okFetch });
    applications.push(app);
    const policy = await FilePolicy.create({
      cwd: root,
      configuredRoots: [],
      cliRoots: [],
      include: [],
      exclude: [],
      protectedPaths: [],
    });
    const input = new PassThrough();
    const output = new PassThrough();
    const frame = new Promise<string>((resolve) =>
      output.once("data", (data) => resolve(String(data))),
    );
    const transport = new StdioServerTransport(input, output);
    await createMcpServer(app, policy).connect(transport);
    input.write(
      `${JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        method: "initialize",
        params: {
          protocolVersion: LATEST_PROTOCOL_VERSION,
          capabilities: {},
          clientInfo: { name: "stdio-test", version: "1" },
        },
      })}\n`,
    );
    const stdout = await frame;
    expect(stdout.endsWith("\n")).toBeTrue();
    expect(() => JSON.parse(stdout.trim())).not.toThrow();
    expect(stdout.trim().split("\n")).toHaveLength(1);
    await transport.close();
  });

  test("delivers through default and explicit bots, aliases, and threads", async () => {
    const requests: Array<{ method: string; body: Record<string, unknown> }> = [];
    const { app, client } = await setup((async (url, init) => {
      requests.push({
        method: String(url).split("/").at(-1) as string,
        body: JSON.parse(String(init?.body)),
      });
      return Response.json({ ok: true, result: { message_id: requests.length } });
    }) as Fetch);
    await app.bots.register("one");
    await app.bots.register("two");
    app.aliases.create({ name: "ops", chatId: "-100", messageThreadId: 42 });

    const first = await client.call("send_text", { to: "ops", payload: { text: "hello" } });
    const second = await client.call("send_text", {
      to: "ops",
      bot: "other_bot",
      messageThreadId: 99,
      payload: { text: "world" },
    });
    expect(first.isError).not.toBeTrue();
    expect(second.isError).not.toBeTrue();
    expect(requests.map(({ body }) => body.message_thread_id)).toEqual([42, 99]);
    expect(requests.map(({ method }) => method)).toEqual(["sendMessage", "sendMessage"]);
  });

  test("returns safe tool errors", async () => {
    const token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
    const { app, client } = await setup((async () =>
      Response.json({ ok: false, description: token }, { status: 400 })) as Fetch);
    await app.bots.register(token);
    const result = await client.call("send_text", { to: "1", payload: { text: "hello" } });
    expect(result.isError).toBeTrue();
    expect(JSON.stringify(result)).not.toContain(token);
  });

  test("supports file_id, URL, and approved paths across media tools", async () => {
    const contentTypes: string[] = [];
    const { app, client, root } = await setup((async (_url, init) => {
      contentTypes.push(init?.body instanceof FormData ? "multipart" : "json");
      return Response.json({ ok: true, result: { message_id: 1 } });
    }) as Fetch);
    await app.bots.register("token");
    const path = join(root, "media.bin");
    await Bun.write(path, "media");

    for (const operation of MESSAGE_CATALOG.filter(({ mediaFields }) => mediaFields.length > 0)) {
      const result = await client.call(toolName(operation.type), {
        to: "1",
        payload: replaceMedia(operation.examplePayload, { source: "path", value: path }),
      });
      expect(result.isError).not.toBeTrue();
    }
    expect(contentTypes).toEqual(
      MESSAGE_CATALOG.filter(({ mediaFields }) => mediaFields.length > 0).map(() => "multipart"),
    );

    expect(
      (
        await client.call("send_photo", {
          to: "1",
          payload: { photo: { source: "file_id", value: "telegram-id" } },
        })
      ).isError,
    ).not.toBeTrue();
    expect(
      (
        await client.call("send_photo", {
          to: "1",
          payload: { photo: { source: "url", value: "https://example.com/photo.jpg" } },
        })
      ).isError,
    ).not.toBeTrue();
  });

  test("rejects denied paths and byte-bearing media before delivery", async () => {
    let deliveries = 0;
    const { app, client, root } = await setup((async () => {
      deliveries++;
      return Response.json({ ok: true, result: { message_id: 1 } });
    }) as Fetch);
    await app.bots.register("token");
    const denied = join(root, ".env");
    await Bun.write(denied, "SECRET=value");
    expect(
      (
        await client.call("send_document", {
          to: "1",
          payload: { document: { source: "path", value: denied } },
        })
      ).isError,
    ).toBeTrue();
    expect(
      (
        await client.call("send_document", {
          to: "1",
          payload: { document: { source: "upload", value: "bytes" } },
        })
      ).isError,
    ).toBeTrue();
    expect(deliveries).toBe(0);
  });
});

test("authorizeMcpPayload canonicalizes nested paths", async () => {
  const root = await mkdtemp(join(tmpdir(), "telesend-mcp-payload-"));
  roots.push(root);
  const path = join(root, "photo.jpg");
  await Bun.write(path, "photo");
  const policy = await FilePolicy.create({
    cwd: root,
    configuredRoots: [],
    cliRoots: [],
    include: [],
    exclude: [],
    protectedPaths: [],
  });
  expect(
    await authorizeMcpPayload({ media: [{ media: { source: "path", value: path } }] }, policy),
  ).toEqual({ media: [{ media: { source: "path", value: path } }] });
});

const okFetch = (async () => Response.json({ ok: true, result: { message_id: 1 } })) as Fetch;

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
