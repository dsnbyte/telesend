import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createApplication } from "../src/app.ts";
import { type CliServices, runCli } from "../src/cli.ts";
import { MESSAGE_CATALOG } from "../src/telegram/catalog.ts";
import type { Fetch } from "../src/telegram/client.ts";

const databases: Database[] = [];
const roots: string[] = [];
afterEach(async () => {
  for (const database of databases.splice(0)) database.close(false);
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function setup() {
  const calls: string[] = [];
  const fetcher = (async (url, init) => {
    const method = String(url).split("/").at(-1) as string;
    calls.push(method);
    if (method === "getMe") {
      return Response.json({
        ok: true,
        result: {
          id: calls.length,
          is_bot: true,
          first_name: "Alert",
          username: `alert_${calls.length}_bot`,
        },
      });
    }
    const body = init?.body instanceof FormData ? init.body : JSON.parse(String(init?.body));
    return Response.json({ ok: true, result: { message_id: calls.length, echoed: body } });
  }) as Fetch;
  const app = await createApplication({ databasePath: ":memory:", fetcher });
  databases.push(app.database);
  const output: string[] = [];
  const errors: string[] = [];
  const starts: string[] = [];
  const services: CliServices = {
    aliases: app.aliases,
    bots: app.bots,
    delivery: app.delivery,
    readSecret: async () => "super-secret-token",
    startRest: async () => {
      starts.push("rest");
    },
    startMcp: async () => {
      starts.push("mcp");
    },
    startRemoteMcp: async () => {
      starts.push("mcp-serve");
    },
  };
  const invoke = (args: string[]) =>
    runCli(args, services, {
      log: (message) => output.push(message),
      error: (message) => errors.push(message),
    });
  return { app, calls, errors, invoke, output, starts };
}

describe("CLI", () => {
  test("prints help and handles unknown commands", async () => {
    const cli = await setup();
    expect(await cli.invoke([])).toBe(0);
    expect(cli.output[0]).toContain("Usage:");
    expect(cli.output[0]).toContain("telesend [--json] msg|message");
    expect(cli.output[0]).toContain("telesend mcp-serve");
    expect(cli.output[0]).toContain("--json           Print JSON instead of human-readable text");
    expect(cli.output[0]).toContain("-V, --version    Print the installed version");
    expect(cli.output[0]).toContain("telesend --version");
    expect(cli.output[0]).toContain("rich-message");
    expect(await cli.invoke(["unknown"])).toBe(1);
    expect(cli.errors.at(-1)).toBe("Error: Unknown command: unknown");
  });

  test("prints the installed version", async () => {
    const cli = await setup();
    const { version } = (await Bun.file(join(import.meta.dir, "../package.json")).json()) as {
      version: string;
    };
    for (const flag of ["--version", "-V", "version"]) {
      expect(await cli.invoke([flag])).toBe(0);
      expect(cli.output.at(-1)).toBe(version);
    }
  });

  test("registers and manages bots without exposing tokens", async () => {
    const cli = await setup();
    expect(await cli.invoke(["bot", "list"])).toBe(0);
    expect(cli.output.at(-1)).toBe("No bots registered.");
    expect(await cli.invoke(["bot", "add"])).toBe(0);
    expect(cli.output.join("\n")).not.toContain("super-secret-token");
    expect(cli.output.at(-1)).toContain("Added bot @alert_1_bot");
    expect(cli.output.at(-1)).toContain("Default      yes");
    expect(await cli.invoke(["bot", "add", "token-in-history"])).toBe(1);
    await cli.invoke(["bot", "list"]);
    expect(cli.output.at(-1)).toContain("USERNAME");
    expect(cli.output.at(-1)).toContain("alert_1_bot");
    expect(cli.output.at(-1)).not.toMatch(/^\s*\{/m);
  });

  test("manages aliases including message threads", async () => {
    const cli = await setup();
    expect(await cli.invoke(["alias", "list"])).toBe(0);
    expect(cli.output.at(-1)).toBe("No aliases registered.");
    await cli.invoke(["alias", "add", "ops", "-100", "--thread-id", "42"]);
    expect(cli.output.at(-1)).toContain("Added alias ops");
    expect(cli.output.at(-1)).toContain("Thread   42");
    await cli.invoke(["alias", "list"]);
    expect(cli.output.at(-1)).toContain("NAME");
    expect(cli.output.at(-1)).toContain("TYPE");
    expect(cli.output.at(-1)).toContain("ops");
    expect(cli.output.at(-1)).toContain("group");
    expect(cli.output.at(-1)).toContain("-100");
    await cli.invoke(["alias", "update", "ops", "--name", "releases", "--thread-id", "99"]);
    expect(cli.app.aliases.find("releases")).toEqual({
      name: "releases",
      chatId: "-100",
      messageThreadId: 99,
      type: "group",
    });
    expect(cli.output.at(-1)).toContain("Updated alias releases");
    await cli.invoke(["alias", "remove", "releases"]);
    expect(cli.output.at(-1)).toContain("Removed alias releases");
    expect(cli.app.aliases.list()).toHaveLength(0);
    await cli.invoke(["alias", "add", "Team Chat", "212711973"]);
    expect(cli.app.aliases.find("Team Chat")).toEqual({
      name: "Team Chat",
      chatId: "212711973",
      messageThreadId: null,
      type: "private",
    });
    expect(cli.output.at(-1)).toContain("Added alias Team Chat");
    expect(cli.output.at(-1)).toContain("private");
  });

  test("sends every catalog type and local files through explicit commands", async () => {
    const cli = await setup();
    await cli.invoke(["bot", "add"]);
    await cli.invoke(["alias", "add", "ops", "-100", "--thread-id", "42"]);
    for (const operation of MESSAGE_CATALOG) {
      expect(
        await cli.invoke([
          "msg",
          operation.type,
          "ops",
          "--data",
          JSON.stringify(operation.examplePayload),
          "--bot",
          "alert_1_bot",
        ]),
      ).toBe(0);
    }
    expect(cli.output.at(-1)).toContain("Message sent");
    expect(cli.output.at(-1)).toContain("Thread      42");
    expect(cli.output.at(-1)).toContain("Message ID  ");

    const root = await mkdtemp(join(tmpdir(), "telesend-cli-"));
    roots.push(root);
    const path = join(root, "report.txt");
    await Bun.write(path, "report");
    expect(await cli.invoke(["message", "document", "ops", path, "--caption", "Report"])).toBe(0);
    expect(await cli.invoke(["send", "document", "ops", path])).toBe(1);
    expect(cli.errors.at(-1)).toBe("Error: Unknown command: send");
  });

  test("applies text format and silent flags after --data", async () => {
    const cli = await setup();
    await cli.invoke(["bot", "add"]);
    expect(
      await cli.invoke([
        "msg",
        "text",
        "-100",
        "--data",
        '{"text":"Hello","parse_mode":"HTML","disable_notification":false}',
        "--parse-mode=md",
        "--silent",
        "--json",
      ]),
    ).toBe(0);
    const result = JSON.parse(cli.output.at(-1) as string) as {
      result: { echoed: Record<string, unknown> };
    };
    expect(result.result.echoed.parse_mode).toBe("MarkdownV2");
    expect(result.result.echoed.disable_notification).toBe(true);
  });

  test("maps supported text parse modes", async () => {
    const cli = await setup();
    await cli.invoke(["bot", "add"]);
    for (const [flag, parseMode] of [
      ["--parse-mode=html", "HTML"],
      ["--parse-mode=markdown", "MarkdownV2"],
      ["--parse-mode=md", "MarkdownV2"],
    ] as const) {
      expect(await cli.invoke(["msg", "text", "-100", "Hello", flag, "--json"])).toBe(0);
      const result = JSON.parse(cli.output.at(-1) as string) as {
        result: { echoed: Record<string, unknown> };
      };
      expect(result.result.echoed.parse_mode).toBe(parseMode);
    }
  });

  test("rejects invalid or unsupported text parse modes", async () => {
    const cli = await setup();
    expect(await cli.invoke(["msg", "text", "-100", "Hello", "--parse-mode=plain"])).toBe(1);
    expect(cli.errors.at(-1)).toBe("Error: --parse-mode must be html, markdown, or md");
    expect(await cli.invoke(["msg", "text", "-100", "Hello", "--parse-mode"])).toBe(1);
    expect(cli.errors.at(-1)).toBe("Error: Use --parse-mode=<html|markdown|md>");
    expect(await cli.invoke(["msg", "photo", "-100", "file.jpg", "--parse-mode=html"])).toBe(1);
    expect(cli.errors.at(-1)).toBe("Error: --parse-mode is only supported for text messages");
  });

  test("prints JSON when --json is set", async () => {
    const cli = await setup();
    expect(await cli.invoke(["alias", "list", "--json"])).toBe(0);
    expect(cli.output.at(-1)).toBe("[]");
    expect(await cli.invoke(["--json", "bot", "add"])).toBe(0);
    const added = JSON.parse(cli.output.at(-1) as string) as {
      telegramId: string;
      name: string;
      username: string;
      isDefault: boolean;
    };
    expect(added).toEqual({
      telegramId: "1",
      name: "Alert",
      username: "alert_1_bot",
      isDefault: true,
    });
    expect(await cli.invoke(["bot", "list", "--json"])).toBe(0);
    expect(JSON.parse(cli.output.at(-1) as string)).toEqual([added]);
    expect(await cli.invoke(["alias", "add", "ops", "-100", "--thread-id", "42", "--json"])).toBe(
      0,
    );
    expect(JSON.parse(cli.output.at(-1) as string)).toEqual({
      name: "ops",
      chatId: "-100",
      messageThreadId: 42,
      type: "group",
    });
    expect(await cli.invoke(["msg", "text", "ops", "Hello", "--json"])).toBe(0);
    const sent = JSON.parse(cli.output.at(-1) as string) as {
      bot: string;
      chatId: string;
      messageThreadId: number;
      messageId: number;
    };
    expect(sent.bot).toBe("alert_1_bot");
    expect(sent.chatId).toBe("-100");
    expect(sent.messageThreadId).toBe(42);
    expect(sent.messageId).toBeGreaterThan(0);
  });

  test("dispatches REST and MCP modes without stdout noise", async () => {
    const cli = await setup();
    await cli.invoke(["serve", "--port", "9000"]);
    await cli.invoke(["mcp", "--allow-path", "/tmp", "--allow-path", "/var/tmp"]);
    await cli.invoke(["mcp-serve", "--host", "127.0.0.1", "--port", "9001"]);
    expect(cli.starts).toEqual(["rest", "mcp", "mcp-serve"]);
    expect(cli.output).toHaveLength(0);
  });
});
