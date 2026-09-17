import { describe, expect, test } from "bun:test";
import { encodeOwnerPasswordHash } from "../src/mcp/owner-password-hash.ts";
import { loadRemoteMcpConfig } from "../src/mcp/remote-config.ts";

describe("remote MCP configuration", () => {
  test("requires an HTTPS public origin and an owner password", () => {
    expect(() => loadRemoteMcpConfig({})).toThrow("TELESEND_MCP_PUBLIC_URL is required");
    expect(() =>
      loadRemoteMcpConfig({
        TELESEND_MCP_OWNER_PASSWORD_HASH: "$argon2id$v=19$m=65536,t=2,p=1$hash",
        TELESEND_MCP_PUBLIC_URL: "http://example.com",
      }),
    ).toThrow("HTTPS origin");
    expect(() =>
      loadRemoteMcpConfig({
        TELESEND_MCP_OWNER_PASSWORD_HASH: "plaintext-secret",
        TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com",
      }),
    ).toThrow("password hash");
    expect(() =>
      loadRemoteMcpConfig({ TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com" }),
    ).toThrow("TELESEND_MCP_OWNER_PASSWORD or TELESEND_MCP_OWNER_PASSWORD_HASH is required");
  });

  test("hashes a plaintext owner password", async () => {
    const config = loadRemoteMcpConfig({
      TELESEND_MCP_OWNER_PASSWORD: "owner-password",
      TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com",
    });

    expect(await Bun.password.verify("owner-password", config.ownerPasswordHash)).toBe(true);
  });

  test("prefers a configured hash over a plaintext owner password", async () => {
    const configuredPassword = "configured-password";
    const config = loadRemoteMcpConfig({
      TELESEND_MCP_OWNER_PASSWORD: "plaintext-password",
      TELESEND_MCP_OWNER_PASSWORD_HASH: await Bun.password.hash(configuredPassword),
      TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com",
    });

    expect(await Bun.password.verify(configuredPassword, config.ownerPasswordHash)).toBe(true);
    expect(await Bun.password.verify("plaintext-password", config.ownerPasswordHash)).toBe(false);
  });

  test("accepts a precomputed hash without including the secret in errors", () => {
    const config = loadRemoteMcpConfig({
      TELESEND_MCP_OWNER_PASSWORD_HASH: "$argon2id$v=19$m=65536,t=2,p=1$hash",
      TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com",
    });
    expect(config.publicUrl.href).toBe("https://mcp.example.com/");
    expect(config.ownerPasswordHash).toStartWith("$argon2id$");
  });

  test("accepts an dotenv-safe encoded hash", () => {
    const hash = "$argon2id$v=19$m=65536,t=2,p=1$salt$digest";
    const config = loadRemoteMcpConfig({
      TELESEND_MCP_OWNER_PASSWORD_HASH: encodeOwnerPasswordHash(hash),
      TELESEND_MCP_PUBLIC_URL: "https://mcp.example.com",
    });

    expect(config.ownerPasswordHash).toBe(hash);
  });
});
