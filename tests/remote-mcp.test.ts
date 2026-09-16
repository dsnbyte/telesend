import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { LATEST_PROTOCOL_VERSION } from "@modelcontextprotocol/server";
import { createApplication } from "../src/app.ts";
import { oauthTesting, TelesendOAuth } from "../src/mcp/oauth.ts";
import { OAuthRepository } from "../src/mcp/oauth-repository.ts";
import {
  createRemoteMcpHandler,
  DEFAULT_REMOTE_MCP_PORT,
  startRemoteMcpServer,
} from "../src/mcp/remote-server.ts";
import type { Fetch } from "../src/telegram/client.ts";

const databases: Database[] = [];
const roots: string[] = [];

afterEach(async () => {
  for (const database of databases.splice(0)) database.close(false);
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function setup(scopes = ["mcp:read", "mcp:send"]) {
  let deliveries = 0;
  const app = await createApplication({
    databasePath: ":memory:",
    fetcher: (async (url) => {
      if (String(url).endsWith("/getMe")) {
        return Response.json({
          ok: true,
          result: { id: 1, is_bot: true, first_name: "Remote", username: "remote_bot" },
        });
      }
      deliveries++;
      return Response.json({ ok: true, result: { message_id: deliveries } });
    }) as Fetch,
  });
  databases.push(app.database);
  const config = {
    ownerPasswordHash: "$argon2id$test",
    publicUrl: new URL("https://mcp.example.com"),
  };
  const repository = new OAuthRepository(app.database);
  repository.registerClient({ id: "client", name: "test", redirectUris: ["https://client/cb"] }, 1);
  repository.createGrant({ id: "grant", clientId: "client", scopes }, 1);
  repository.storeTokens({
    accessDigest: oauthTesting.digest("access-token"),
    accessExpiresAt: 4_000_000_000,
    grantId: "grant",
    refreshDigest: oauthTesting.digest("refresh-token"),
    refreshExpiresAt: 4_100_000_000,
  });
  const oauth = new TelesendOAuth(repository, config, { now: () => 100 });
  return {
    app,
    config,
    deliveries: () => deliveries,
    handler: createRemoteMcpHandler(app, oauth, config),
    repository,
  };
}

function mcpRequest(body: Record<string, unknown>, token = "access-token") {
  return new Request("https://local/mcp", {
    method: "POST",
    headers: {
      accept: "application/json, text/event-stream",
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    body: JSON.stringify(body),
  });
}

describe("remote MCP HTTP", () => {
  test("requires bearer auth and serves MCP initialization", async () => {
    const { handler } = await setup();
    const unauthorized = await handler.fetch(new Request("https://local/mcp", { method: "POST" }));
    expect(unauthorized.status).toBe(401);
    expect(unauthorized.headers.get("www-authenticate")).toContain("resource_metadata");
    expect(unauthorized.headers.get("www-authenticate")).not.toContain("scope=");

    const response = await handler.fetch(
      mcpRequest({
        jsonrpc: "2.0",
        id: 1,
        method: "initialize",
        params: {
          protocolVersion: LATEST_PROTOCOL_VERSION,
          capabilities: {},
          clientInfo: { name: "http-test", version: "1" },
        },
      }),
    );
    expect(response.status).toBe(200);
    expect(await mcpJson(response)).toMatchObject({
      jsonrpc: "2.0",
      id: 1,
      result: { serverInfo: { name: "telesend" } },
    });
  });

  test("returns safe route, method, origin, and request-size failures", async () => {
    const { handler } = await setup();
    expect((await handler.fetch(new Request("https://local/missing"))).status).toBe(404);
    expect(
      (
        await handler.fetch(
          new Request("https://local/mcp", {
            method: "PUT",
            headers: { authorization: "Bearer access-token" },
          }),
        )
      ).status,
    ).toBe(405);
    expect(
      (
        await handler.fetch(
          new Request("https://local/mcp", {
            headers: { origin: "https://attacker.example" },
          }),
        )
      ).status,
    ).toBe(403);
    expect(
      (
        await handler.fetch(
          new Request("https://local/mcp", {
            method: "POST",
            headers: { "content-length": String(2 * 1024 * 1024) },
          }),
        )
      ).status,
    ).toBe(413);
  });

  test("enforces scope, rejects paths, delivers remotely, and honors revocation", async () => {
    const readOnly = await setup(["mcp:read"]);
    await readOnly.app.bots.register("token");
    const denied = (await mcpJson(
      await readOnly.handler.fetch(
        mcpRequest({
          jsonrpc: "2.0",
          id: 2,
          method: "tools/call",
          params: { name: "send_text", arguments: { to: "1", payload: { text: "denied" } } },
        }),
      ),
    )) as { result?: { isError?: boolean } };
    expect(denied.result?.isError).toBeTrue();
    expect(readOnly.deliveries()).toBe(0);

    const allowed = await setup();
    await allowed.app.bots.register("token");
    const sent = (await mcpJson(
      await allowed.handler.fetch(
        mcpRequest({
          jsonrpc: "2.0",
          id: 3,
          method: "tools/call",
          params: { name: "send_text", arguments: { to: "1", payload: { text: "hello" } } },
        }),
      ),
    )) as { result?: { isError?: boolean } };
    expect(sent.result?.isError).not.toBeTrue();
    expect(allowed.deliveries()).toBe(1);

    const path = (await mcpJson(
      await allowed.handler.fetch(
        mcpRequest({
          jsonrpc: "2.0",
          id: 4,
          method: "tools/call",
          params: {
            name: "send_document",
            arguments: {
              to: "1",
              payload: { document: { source: "path", value: "/etc/passwd" } },
            },
          },
        }),
      ),
    )) as { error?: unknown; result?: { isError?: boolean } };
    expect(path.error !== undefined || path.result?.isError === true).toBeTrue();
    expect(allowed.deliveries()).toBe(1);

    allowed.repository.revokeGrantByToken(oauthTesting.digest("access-token"), 101);
    const revoked = await allowed.handler.fetch(
      mcpRequest({ jsonrpc: "2.0", id: 5, method: "tools/list", params: {} }),
    );
    expect(revoked.status).toBe(401);
    expect(await revoked.text()).not.toContain("access-token");
  });

  test("does not expose submitted OAuth or bearer secrets in failures", async () => {
    const { handler } = await setup();
    const bearer = "secret-invalid-bearer";
    const invalidBearer = await handler.fetch(
      mcpRequest({ jsonrpc: "2.0", id: 1, method: "tools/list", params: {} }, bearer),
    );
    expect(await invalidBearer.text()).not.toContain(bearer);

    const password = "secret-owner-password";
    const invalidOwner = await handler.fetch(
      new Request("https://local/authorize", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ password }),
      }),
    );
    expect(await invalidOwner.text()).not.toContain(password);
  });

  test("starts on loopback and reports local and public addresses", async () => {
    const { app, config } = await setup();
    const starts: Array<{ hostname?: string; port?: number }> = [];
    const logs: string[] = [];
    startRemoteMcpServer(
      app,
      {},
      {
        config,
        log: (message) => logs.push(message),
        serve: (options) => {
          starts.push({
            ...(options.hostname === undefined ? {} : { hostname: options.hostname }),
            ...(options.port === undefined ? {} : { port: options.port }),
          });
          return { url: new URL("http://127.0.0.1:4321"), stop: () => undefined };
        },
      },
    );
    expect(starts).toEqual([{ hostname: "127.0.0.1", port: DEFAULT_REMOTE_MCP_PORT }]);
    expect(logs[0]).toContain("http://127.0.0.1:4321/");
    expect(logs[0]).toContain("https://mcp.example.com/mcp");
  });

  test("completes OAuth and remote delivery end to end on a temporary database", async () => {
    const root = await mkdtemp(join(tmpdir(), "telesend-remote-e2e-"));
    roots.push(root);
    let deliveries = 0;
    const app = await createApplication({
      databasePath: join(root, "telesend.db"),
      fetcher: (async (url) => {
        if (String(url).endsWith("/getMe")) {
          return Response.json({
            ok: true,
            result: { id: 1, is_bot: true, first_name: "E2E", username: "e2e_bot" },
          });
        }
        deliveries++;
        return Response.json({ ok: true, result: { message_id: deliveries } });
      }) as Fetch,
    });
    databases.push(app.database);
    await app.bots.register("telegram-token");
    const config = {
      ownerPasswordHash: "$argon2id$test",
      publicUrl: new URL("https://mcp.example.com"),
    };
    let sequence = 0;
    const oauth = new TelesendOAuth(new OAuthRepository(app.database), config, {
      now: () => 2_000_000_000,
      randomToken: () => `e2e-${++sequence}`,
      verifyPassword: async (password) => password === "owner-password",
    });
    const handler = createRemoteMcpHandler(app, oauth, config);

    const registration = await handler.fetch(
      new Request("https://local/register", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          client_name: "E2E client",
          redirect_uris: ["https://client.example/callback"],
          token_endpoint_auth_method: "none",
        }),
      }),
    );
    const { client_id: clientId } = (await registration.json()) as { client_id: string };
    const verifier = "v".repeat(43);
    const consent = await handler.fetch(
      new Request("https://local/authorize", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          response_type: "code",
          client_id: clientId,
          redirect_uri: "https://client.example/callback",
          scope: "mcp:read mcp:send",
          code_challenge: createHash("sha256").update(verifier).digest("base64url"),
          code_challenge_method: "S256",
          resource: "https://mcp.example.com/mcp",
          password: "owner-password",
        }),
        redirect: "manual",
      }),
    );
    const code = new URL(consent.headers.get("location") as string).searchParams.get(
      "code",
    ) as string;
    const tokenResponse = await handler.fetch(
      new Request("https://local/token", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          client_id: clientId,
          code,
          redirect_uri: "https://client.example/callback",
          code_verifier: verifier,
        }),
      }),
    );
    const { access_token: accessToken } = (await tokenResponse.json()) as {
      access_token: string;
    };

    const sent = (await mcpJson(
      await handler.fetch(
        mcpRequest(
          {
            jsonrpc: "2.0",
            id: 10,
            method: "tools/call",
            params: {
              name: "send_text",
              arguments: { to: "1", payload: { text: "from remote MCP" } },
            },
          },
          accessToken,
        ),
      ),
    )) as { result?: { isError?: boolean } };
    expect(sent.result?.isError).not.toBeTrue();
    expect(deliveries).toBe(1);

    await handler.fetch(
      mcpRequest(
        {
          jsonrpc: "2.0",
          id: 11,
          method: "tools/call",
          params: {
            name: "send_document",
            arguments: {
              to: "1",
              payload: { document: { source: "path", value: join(root, "secret.txt") } },
            },
          },
        },
        accessToken,
      ),
    );
    expect(deliveries).toBe(1);

    await handler.fetch(
      new Request("https://local/revoke", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ token: accessToken }),
      }),
    );
    expect(
      (
        await handler.fetch(
          mcpRequest({ jsonrpc: "2.0", id: 12, method: "tools/list", params: {} }, accessToken),
        )
      ).status,
    ).toBe(401);
  });
});

async function mcpJson(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!response.headers.get("content-type")?.includes("text/event-stream")) {
    return JSON.parse(text);
  }
  const data = text
    .split("\n")
    .find((line) => line.startsWith("data: "))
    ?.slice(6);
  return JSON.parse(data ?? "null");
}
