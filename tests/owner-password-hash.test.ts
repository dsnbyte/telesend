import { describe, expect, test } from "bun:test";
import {
  decodeOwnerPasswordHash,
  encodeOwnerPasswordHash,
  generateOwnerPasswordHash,
} from "../src/mcp/owner-password-hash.ts";

describe("owner password hash generator", () => {
  test("hashes a confirmed password", async () => {
    const values = ["owner-password", "owner-password"];
    const hash = await generateOwnerPasswordHash(async () => values.shift() ?? "");

    expect(await Bun.password.verify("owner-password", hash)).toBeTrue();
  });

  test("encodes hashes in an dotenv-safe representation", async () => {
    const hash = await Bun.password.hash("owner-password");
    const encoded = encodeOwnerPasswordHash(hash);

    expect(encoded).toMatch(/^[A-Za-z0-9_-]+$/);
    expect(decodeOwnerPasswordHash(encoded)).toBe(hash);
  });

  test("rejects an empty or mismatched password", async () => {
    await expect(generateOwnerPasswordHash(async () => "")).rejects.toThrow(
      "Owner password is required",
    );

    const values = ["owner-password", "other-password"];
    await expect(generateOwnerPasswordHash(async () => values.shift() ?? "")).rejects.toThrow(
      "Owner password confirmation does not match",
    );
  });
});
