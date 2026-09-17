import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PassThrough } from "node:stream";
import { LATEST_PROTOCOL_VERSION } from "@modelcontextprotocol/server";
import { StdioServerTransport } from "@modelcontextprotocol/server/stdio";
import { type Application, createApplication } from "../src/app.ts";
import type { MethodDoc } from "../src/mcp/docs.ts";
import { FilePolicy } from "../src/mcp/file-policy.ts";
import {
  authorizeMcpPayload,
  authorizeRemoteMcpPayload,
  createMcpServer,
  createRemoteMcpServer,
} from "../src/mcp/server.ts";
import type { Fetch } from "../src/telegram/client.ts";
import { VERSION } from "../src/version.ts";
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
  test("advertises instructions, distinct send tools, and lookup tools", async () => {
    const { client } = await setup(okFetch);
    expect(client.initializeResult.serverInfo?.name).toBe("telesend");
    expect(client.initializeResult.serverInfo?.version).toBe(VERSION);
    expect(client.initializeResult.instructions).toContain(
      "Pick the send_* tool that matches the content",
    );
    expect(client.initializeResult.instructions).toContain("send_text");
    expect(client.initializeResult.instructions).toContain("list_aliases");
    expect(client.initializeResult.instructions).toContain("type group or private");
    expect(client.initializeResult.instructions).toContain("payload.disable_notification: true");
    expect(client.initializeResult.instructions).toContain(
      "If the user intends to send a message to all aliases or all groups",
    );
    expect(client.initializeResult.instructions).toContain(
      "Call get_telegram_parameter_doc only for advanced Telegram options",
    );
    expect(client.initializeResult.instructions).not.toContain("plain text (add draft_id");

    const result = await client.request("tools/list", {});
    const tools = (
      result as {
        tools: Array<{
          name: string;
          description?: string;
          inputSchema?: {
            properties?: {
              payload?: {
                properties?: Record<
                  string,
                  {
                    type?: string;
                    properties?: { source?: unknown; value?: { description?: string } };
                  }
                >;
              };
            };
          };
        }>;
      }
    ).tools;
    const sendTools = tools.filter(({ name }) => name.startsWith("send_"));
    expect(tools.find(({ name }) => name === "list_aliases")?.description).toContain(
      "type (`group` or `private`)",
    );
    expect(tools.map(({ name }) => name)).toEqual([
      "list_aliases",
      "list_bots",
      "get_telegram_parameter_doc",
      "send_text",
      "send_rich_message",
      "send_media",
      "send_media_group",
      "send_location",
      "send_contact",
      "send_interactive",
      "send_invoice",
    ]);
    expect(tools.some(({ name }) => name === "telegram_method")).toBeFalse();

    const descriptions = sendTools.map(({ description }) => description);
    expect(new Set(descriptions).size).toBe(descriptions.length);
    for (const description of descriptions) {
      expect(description).not.toContain("get_telegram_parameter_doc");
    }
    expect(tools.find(({ name }) => name === "send_text")?.description).toContain(
      'parse_mode: "HTML" or "MarkdownV2"',
    );
    expect(tools.find(({ name }) => name === "send_text")?.description).toContain("draft");

    expect(
      tools.find(({ name }) => name === "send_text")?.inputSchema?.properties?.payload?.properties
        ?.text?.type,
    ).toBe("string");
    expect(
      sourceEnum(tools.find(({ name }) => name === "send_media")?.inputSchema, "file"),
    ).toEqual(["path", "url", "file_id"]);
    expect(
      sourceEnum(tools.find(({ name }) => name === "send_media")?.inputSchema, "photo"),
    ).toEqual(["path", "file_id"]);
  });

  test("annotates tools and exposes a remote-safe media schema", async () => {
    const app = await createApplication({ databasePath: ":memory:", fetcher: okFetch });
    applications.push(app);
    const client = await TestMcpClient.connect(
      createRemoteMcpServer(app, ["mcp:read", "mcp:send"]),
    );
    expect(client.initializeResult.instructions).toContain(
      "ask for confirmation and wait for an explicit affirmative response before sending",
    );
    const result = await client.request("tools/list", {});
    const tools = (
      result as {
        tools: Array<{
          name: string;
          annotations?: { readOnlyHint?: boolean; idempotentHint?: boolean };
          inputSchema?: {
            properties?: {
              payload?: {
                properties?: Record<
                  string,
                  { properties?: { source?: unknown; value?: { description?: string } } }
                >;
              };
            };
          };
        }>;
      }
    ).tools;
    expect(tools.find(({ name }) => name === "list_aliases")?.annotations?.readOnlyHint).toBeTrue();
    expect(tools.find(({ name }) => name === "send_text")?.annotations).toMatchObject({
      readOnlyHint: false,
      idempotentHint: false,
    });
    expect(
      sourceEnum(tools.find(({ name }) => name === "send_media")?.inputSchema, "file"),
    ).toEqual(["url", "file_id"]);
    expect(
      sourceEnum(tools.find(({ name }) => name === "send_media")?.inputSchema, "photo"),
    ).toEqual(["file_id"]);
    expect(
      tools.find(({ name }) => name === "send_media")?.inputSchema?.properties?.payload?.properties
        ?.file?.properties?.value?.description,
    ).toContain("Chat attachments and generated files cannot be uploaded directly");
  });

  test("enforces remote scopes before reading or delivering", async () => {
    let deliveries = 0;
    const app = await createApplication({
      databasePath: ":memory:",
      fetcher: (async (url) => {
        if (String(url).endsWith("/getMe")) {
          return Response.json({
            ok: true,
            result: { id: 1, is_bot: true, first_name: "bot", username: "bot" },
          });
        }
        deliveries++;
        return Response.json({ ok: true, result: { message_id: 1 } });
      }) as Fetch,
    });
    applications.push(app);
    await app.bots.register("token");
    const client = await TestMcpClient.connect(createRemoteMcpServer(app, ["mcp:read"]));
    expect((await client.call("list_bots", {})).isError).not.toBeTrue();
    expect(
      (
        await client.call("send_text", {
          to: "1",
          payload: { text: "denied" },
        })
      ).isError,
    ).toBeTrue();
    expect(deliveries).toBe(0);
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

  test("lists aliases and bots without secrets", async () => {
    const token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
    const { app, client } = await setup(okFetch);
    await app.bots.register(token);
    app.aliases.create({ name: "didin", chatId: "212711973" });
    app.aliases.create({ name: "Team Chat", chatId: "-1002603419700" });

    const aliases = await client.call("list_aliases", {});
    const bots = await client.call("list_bots", {});
    expect(aliases.structuredContent).toEqual({
      aliases: [
        { name: "didin", chatId: "212711973", messageThreadId: null, type: "private" },
        { name: "Team Chat", chatId: "-1002603419700", messageThreadId: null, type: "group" },
      ],
    });
    expect(bots.structuredContent).toEqual({
      bots: [
        {
          telegramId: "1",
          name: "default",
          username: "default_bot",
          isDefault: true,
        },
      ],
    });
    expect(JSON.stringify(bots)).not.toContain(token);
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

    const singleMedia = [
      "photo",
      "video",
      "animation",
      "audio",
      "document",
      "sticker",
      "voice",
      "video_note",
      "live_photo",
    ];
    for (const type of singleMedia) {
      const payload: Record<string, unknown> = {
        type,
        file: { source: "path", value: path },
      };
      if (type === "live_photo") {
        payload.photo = { source: "path", value: path };
      }
      const result = await client.call("send_media", { to: "1", payload });
      expect(result.isError).not.toBeTrue();
    }
    const groupResult = await client.call("send_media_group", {
      to: "1",
      payload: { media: [{ type: "photo", media: { source: "path", value: path } }] },
    });
    expect(groupResult.isError).not.toBeTrue();

    const paidResult = await client.call("send_media_group", {
      to: "1",
      payload: {
        star_count: 1,
        media: [{ type: "photo", media: { source: "path", value: path } }],
      },
    });
    expect(paidResult.isError).not.toBeTrue();

    expect(contentTypes).toEqual(
      [...singleMedia, "media_group", "paid_media"].map(() => "multipart"),
    );

    expect(
      (
        await client.call("send_media", {
          to: "1",
          payload: { type: "photo", file: { source: "file_id", value: "telegram-id" } },
        })
      ).isError,
    ).not.toBeTrue();
    expect(
      (
        await client.call("send_media", {
          to: "1",
          payload: {
            type: "photo",
            file: { source: "url", value: "https://example.com/photo.jpg" },
          },
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
        await client.call("send_media", {
          to: "1",
          payload: { type: "document", file: { source: "path", value: denied } },
        })
      ).isError,
    ).toBeTrue();
    expect(
      (
        await client.call("send_media", {
          to: "1",
          payload: { type: "document", file: { source: "upload", value: "bytes" } },
        })
      ).isError,
    ).toBeTrue();
    expect(deliveries).toBe(0);
  });

  test("get_telegram_parameter_doc returns docs for methods and types", async () => {
    const { client } = await setup(okFetch);
    const doc1 = await client.call("get_telegram_parameter_doc", { method: "sendMessage" });
    expect(doc1.isError).not.toBeTrue();
    const content1 = doc1.structuredContent as unknown as MethodDoc;
    expect(content1.method).toBe("sendMessage");
    expect(content1.mcpTool).toBe("send_text");
    expect(content1.advancedParams.some((p) => p.name === "reply_markup")).toBeTrue();

    const doc2 = await client.call("get_telegram_parameter_doc", { method: "video" });
    expect(doc2.isError).not.toBeTrue();
    const content2 = doc2.structuredContent as unknown as MethodDoc;
    expect(content2.method).toBe("sendVideo");

    const err = await client.call("get_telegram_parameter_doc", { method: "nonexistent" });
    expect(err.isError).toBeTrue();
    expect(JSON.stringify(err)).toContain("not_found");
  });

  test("routes grouped tools correctly based on payload parameters", async () => {
    const calls: Array<{ method: string; body: Record<string, unknown> }> = [];
    const { app, client } = await setup((async (url, init) => {
      calls.push({
        method: String(url).split("/").at(-1) as string,
        body: JSON.parse(String(init?.body)),
      });
      return Response.json({ ok: true, result: { message_id: calls.length } });
    }) as Fetch);
    await app.bots.register("token");

    // 1. send_text draft routing
    await client.call("send_text", { to: "1", payload: { draft_id: 123 } });
    expect(calls.at(-1)?.method).toBe("sendMessageDraft");
    expect(calls.at(-1)?.body.draft_id).toBe(123);

    // 2. send_rich_message normal vs draft
    await client.call("send_rich_message", {
      to: "1",
      payload: { rich_message: { markdown: "hi" } },
    });
    expect(calls.at(-1)?.method).toBe("sendRichMessage");
    await client.call("send_rich_message", {
      to: "1",
      payload: { draft_id: 1, rich_message: { markdown: "hi" } },
    });
    expect(calls.at(-1)?.method).toBe("sendRichMessageDraft");

    // 3. send_location normal vs venue
    await client.call("send_location", { to: "1", payload: { latitude: 1.0, longitude: 2.0 } });
    expect(calls.at(-1)?.method).toBe("sendLocation");
    await client.call("send_location", {
      to: "1",
      payload: { latitude: 1.0, longitude: 2.0, title: "T", address: "A" },
    });
    expect(calls.at(-1)?.method).toBe("sendVenue");

    // 4. send_interactive: poll, checklist, dice, game
    await client.call("send_interactive", {
      to: "1",
      payload: { type: "poll", question: "Q?", options: [{ text: "O1" }] },
    });
    expect(calls.at(-1)?.method).toBe("sendPoll");
    await client.call("send_interactive", {
      to: "1",
      payload: {
        type: "checklist",
        business_connection_id: "bc",
        checklist: { title: "T", tasks: [] },
      },
    });
    expect(calls.at(-1)?.method).toBe("sendChecklist");
    await client.call("send_interactive", { to: "1", payload: { type: "dice", emoji: "🎯" } });
    expect(calls.at(-1)?.method).toBe("sendDice");
    await client.call("send_interactive", {
      to: "1",
      payload: { type: "game", game_short_name: "testgame" },
    });
    expect(calls.at(-1)?.method).toBe("sendGame");

    // 5. send_contact & send_invoice
    await client.call("send_contact", {
      to: "1",
      payload: { phone_number: "+12345", first_name: "John" },
    });
    expect(calls.at(-1)?.method).toBe("sendContact");
    await client.call("send_invoice", {
      to: "1",
      payload: {
        title: "T",
        description: "D",
        payload: "p",
        currency: "XTR",
        prices: [{ label: "L", amount: 1 }],
      },
    });
    expect(calls.at(-1)?.method).toBe("sendInvoice");
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

test("authorizeRemoteMcpPayload rejects nested paths and preserves remote sources", async () => {
  await expect(
    authorizeRemoteMcpPayload({ media: [{ media: { source: "path", value: "/tmp/file" } }] }),
  ).rejects.toThrow("Remote MCP cannot use server filesystem paths; use a public HTTPS URL");
  await expect(
    authorizeRemoteMcpPayload({ photo: { source: "upload", value: "image bytes" } }),
  ).rejects.toThrow("chat attachments and generated files cannot be uploaded directly");
  expect(
    await authorizeRemoteMcpPayload({
      photo: { source: "url", value: "https://example.com/photo.jpg" },
      thumb: { source: "file_id", value: "telegram-id" },
    }),
  ).toEqual({
    photo: { source: "url", value: "https://example.com/photo.jpg" },
    thumb: { source: "file_id", value: "telegram-id" },
  });
});

const okFetch = (async () => Response.json({ ok: true, result: { message_id: 1 } })) as Fetch;

function sourceEnum(
  schema:
    | {
        properties?: {
          payload?: { properties?: Record<string, { properties?: { source?: unknown } }> };
        };
      }
    | undefined,
  field = "file",
) {
  const prop =
    schema?.properties?.payload?.properties?.[field] ??
    Object.values(schema?.properties?.payload?.properties ?? {})[0];
  const source = prop?.properties?.source;
  if (source && typeof source === "object" && "enum" in source) {
    return (source as { enum: string[] }).enum;
  }
  if (source && typeof source === "object" && "anyOf" in source) {
    return (source as { anyOf: Array<{ const?: string }> }).anyOf.map((entry) => entry.const);
  }
  return source;
}
