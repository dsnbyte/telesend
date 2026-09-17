import { describe, expect, test } from "bun:test";
import { generateOwnerPasswordHash } from "../src/mcp/owner-password-hash.ts";

describe("owner password hash generator", () => {
  test("hashes a confirmed password", async () => {
    const values = ["owner-password", "owner-password"];
    const hash = await generateOwnerPasswordHash(async () => values.shift() ?? "");

    expect(await Bun.password.verify("owner-password", hash)).toBeTrue();
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
