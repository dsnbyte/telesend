import type { Database } from "bun:sqlite";
import { afterEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { openDatabase } from "../src/db/database.ts";
import { TelesendOAuth } from "../src/mcp/oauth.ts";
import { OAuthRepository } from "../src/mcp/oauth-repository.ts";

const databases: Database[] = [];

afterEach(() => {
  for (const database of databases.splice(0)) database.close(false);
});

async function setup() {
  const database = await openDatabase(":memory:");
  databases.push(database);
  let sequence = 0;
  const oauth = new TelesendOAuth(
    new OAuthRepository(database),
    {
      ownerPasswordHash: "$argon2id$test",
      publicUrl: new URL("https://mcp.example.com"),
    },
    {
      now: () => 1_000,
      randomToken: () => `token-${++sequence}`,
      verifyPassword: async (password) => password === "owner-password",
    },
  );
  return { oauth };
}

async function register(oauth: TelesendOAuth) {
  const response = await oauth.handle(
    new Request("https://local/register", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        client_name: "Claude",
        redirect_uris: ["https://claude.ai/api/mcp/auth_callback"],
        token_endpoint_auth_method: "none",
      }),
    }),
  );
  return (await response?.json()) as { client_id: string };
}

function authorizationParams(clientId: string, verifier: string) {
  return new URLSearchParams({
    response_type: "code",
    client_id: clientId,
    redirect_uri: "https://claude.ai/api/mcp/auth_callback",
    scope: "mcp:read mcp:send",
    state: "state",
    code_challenge: createHash("sha256").update(verifier).digest("base64url"),
    code_challenge_method: "S256",
    resource: "https://mcp.example.com/mcp",
  });
}

describe("remote MCP OAuth", () => {
  test("publishes discovery metadata and registers exact redirect URIs", async () => {
    const { oauth } = await setup();
    const protectedMetadata = await oauth.handle(
      new Request("https://local/.well-known/oauth-protected-resource/mcp"),
    );
    expect(await protectedMetadata?.json()).toMatchObject({
      resource: "https://mcp.example.com/mcp",
      authorization_servers: ["https://mcp.example.com"],
    });
    const serverMetadata = await oauth.handle(
      new Request("https://local/.well-known/oauth-authorization-server"),
    );
    expect(await serverMetadata?.json()).toMatchObject({
      authorization_endpoint: "https://mcp.example.com/authorize",
      registration_endpoint: "https://mcp.example.com/register",
    });
    expect(await register(oauth)).toMatchObject({ client_id: "client_token-1" });
    const invalid = await oauth.handle(
      new Request("https://local/register", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ redirect_uris: ["http://attacker.example/callback"] }),
      }),
    );
    expect(invalid?.status).toBe(400);
    expect(await invalid?.json()).toMatchObject({ error: "invalid_redirect_uri" });
  });

  test("requires owner approval and exchanges a PKCE code", async () => {
    const { oauth } = await setup();
    const { client_id: clientId } = await register(oauth);
    const verifier = "v".repeat(43);
    const params = authorizationParams(clientId, verifier);
    const consent = await oauth.handle(new Request(`https://local/authorize?${params.toString()}`));
    expect(consent?.status).toBe(200);
    expect(await consent?.text()).toContain("Authorize Claude");

    const denied = await oauth.handle(
      new Request("https://local/authorize", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ ...Object.fromEntries(params), password: "wrong" }),
      }),
    );
    expect(denied?.status).toBe(401);

    const approved = await oauth.handle(
      new Request("https://local/authorize", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          ...Object.fromEntries(params),
          password: "owner-password",
        }),
        redirect: "manual",
      }),
    );
    const callback = new URL(approved?.headers.get("location") as string);
    expect(callback.searchParams.get("state")).toBe("state");
    const code = callback.searchParams.get("code") as string;

    const invalidPkce = await oauth.handle(
      new Request("https://local/token", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          client_id: clientId,
          code,
          redirect_uri: "https://claude.ai/api/mcp/auth_callback",
          code_verifier: "x".repeat(43),
        }),
      }),
    );
    expect(await invalidPkce?.json()).toMatchObject({ error: "invalid_grant" });
  });

  test("issues, refreshes, verifies, and revokes scoped tokens", async () => {
    const { oauth } = await setup();
    const { client_id: clientId } = await register(oauth);
    const verifier = "v".repeat(43);
    const params = authorizationParams(clientId, verifier);
    const approved = await oauth.handle(
      new Request("https://local/authorize", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          ...Object.fromEntries(params),
          password: "owner-password",
        }),
        redirect: "manual",
      }),
    );
    const code = new URL(approved?.headers.get("location") as string).searchParams.get(
      "code",
    ) as string;
    const tokenResponse = await oauth.handle(
      new Request("https://local/token", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          client_id: clientId,
          code,
          redirect_uri: "https://claude.ai/api/mcp/auth_callback",
          code_verifier: verifier,
        }),
      }),
    );
    const tokens = (await tokenResponse?.json()) as {
      access_token: string;
      refresh_token: string;
    };
    expect(await oauth.verifyAccessToken(tokens.access_token)).toMatchObject({
      clientId,
      scopes: ["mcp:read", "mcp:send"],
    });

    const refreshed = await oauth.handle(
      new Request("https://local/token", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "refresh_token",
          client_id: clientId,
          refresh_token: tokens.refresh_token,
        }),
      }),
    );
    const next = (await refreshed?.json()) as { access_token: string };
    const revoked = await oauth.handle(
      new Request("https://local/revoke", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ token: next.access_token }),
      }),
    );
    expect(revoked?.status).toBe(200);
    await expect(oauth.verifyAccessToken(next.access_token)).rejects.toThrow("invalid or expired");
  });
});
