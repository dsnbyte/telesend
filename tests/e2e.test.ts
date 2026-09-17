import { expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createApplication } from "../src/app.ts";
import { type CliServices, runCli } from "../src/cli.ts";
import { FilePolicy } from "../src/mcp/file-policy.ts";
import { createMcpServer } from "../src/mcp/server.ts";
import { createRestHandler } from "../src/rest/server.ts";
import type { Fetch } from "../src/telegram/client.ts";
import { TestMcpClient } from "./helpers/mcp-client.ts";

test("CLI, REST, and MCP share bots, aliases, threads, media, and safe failures", async () => {
  const root = await mkdtemp(join(tmpdir(), "telesend-e2e-"));
  const databasePath = join(root, "telesend.db");
  const calls: Array<{ token: string; method: string; body: BodyInit | null | undefined }> = [];
  const alphaToken = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZalpha";
  const betaToken = "987654321:ABCDEFGHIJKLMNOPQRSTUVWXYZbeta";
  const fetcher = (async (url, init) => {
    const match = String(url).match(/\/bot([^/]+)\/([^/]+)$/);
    const token = match?.[1] ?? "";
    const method = match?.[2] ?? "";
    if (method === "getMe") {
      const alpha = token === alphaToken;
      return Response.json({
        ok: true,
        result: {
          id: alpha ? 1 : 2,
          is_bot: true,
          first_name: alpha ? "Alpha" : "Beta",
          username: alpha ? "alpha_bot" : "beta_bot",
        },
      });
    }
    calls.push({ token, method, body: init?.body });
    const text = typeof init?.body === "string" ? init.body : "";
    if (text.includes("force-error")) {
      return Response.json({ ok: false, description: `rejected ${token}` }, { status: 400 });
    }
    return Response.json({ ok: true, result: { message_id: calls.length } });
  }) as Fetch;
  const app = await createApplication({ databasePath, fetcher });
  const output: string[] = [];
  const errors: string[] = [];
  const io = {
    log: (message: string) => output.push(message),
    error: (message: string) => errors.push(message),
  };
  const services = (token: string): CliServices => ({
    aliases: app.aliases,
    bots: app.bots,
    delivery: app.delivery,
    readSecret: async () => token,
    startMcp: async () => {},
    startRemoteMcp: async () => {},
    startRest: async () => {},
  });

  try {
    expect(await runCli(["bot", "add"], services(alphaToken), io)).toBe(0);
    expect(await runCli(["bot", "add"], services(betaToken), io)).toBe(0);
    expect(
      await runCli(
        ["alias", "add", "ops", "-100123", "--thread-id", "42"],
        services(alphaToken),
        io,
      ),
    ).toBe(0);
    expect(await runCli(["msg", "text", "ops", "from-cli"], services(alphaToken), io)).toBe(0);

    const rest = createRestHandler(app, await Bun.password.hash("test-api-key"));
    const form = new FormData();
    form.set("to", "ops");
    form.set("bot", "beta_bot");
    form.set("messageThreadId", "77");
    form.set("photo", new File(["image"], "photo.jpg", { type: "image/jpeg" }));
    const restResponse = await rest(
      new Request("http://localhost/messages/photo", {
        method: "POST",
        headers: { "x-api-key": "test-api-key" },
        body: form,
      }),
    );
    expect(restResponse.status).toBe(200);

    const document = join(root, "report.pdf");
    await Bun.write(document, "report");
    const policy = await FilePolicy.create({
      cwd: root,
      configuredRoots: [],
      cliRoots: [],
      include: [],
      exclude: [],
      protectedPaths: [databasePath],
    });
    const mcp = await TestMcpClient.connect(createMcpServer(app, policy));
    const aliases = await mcp.call("list_aliases", {});
    expect(aliases.structuredContent).toEqual({
      aliases: [{ name: "ops", chatId: "-100123", messageThreadId: 42 }],
    });
    const mediaResult = await mcp.call("send_document", {
      to: "ops",
      payload: { document: { source: "path", value: document } },
    });
    expect(mediaResult.isError).not.toBeTrue();

    const errorResult = await mcp.call("send_text", {
      to: "ops",
      payload: { text: "force-error" },
    });
    expect(errorResult.isError).toBeTrue();
    expect(JSON.stringify(errorResult)).not.toContain(alphaToken);

    expect(calls.map(({ token }) => token)).toEqual([
      alphaToken,
      betaToken,
      alphaToken,
      alphaToken,
    ]);
    expect(calls.map(({ method }) => method)).toEqual([
      "sendMessage",
      "sendPhoto",
      "sendDocument",
      "sendMessage",
    ]);
    expect(JSON.parse(String(calls[0]?.body)).message_thread_id).toBe(42);
    const restBody = calls[1]?.body;
    expect(restBody).toBeInstanceOf(FormData);
    expect((restBody as FormData).get("message_thread_id")).toBe("77");
    expect(calls[2]?.body).toBeInstanceOf(FormData);
    expect(errors).toEqual([]);
  } finally {
    app.database.close(false);
    await Bun.$`rm -rf ${root}`.quiet();
  }
});
