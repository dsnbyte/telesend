import { describe, expect, test } from "bun:test";
import { generateApiKeyHash } from "../src/rest/api-key-hash.ts";

describe("API key hash generator", () => {
  test("hashes a confirmed API key", async () => {
    const values = ["api-key", "api-key"];
    const hash = await generateApiKeyHash(async () => values.shift() ?? "");

    expect(await Bun.password.verify("api-key", hash)).toBeTrue();
  });

  test("rejects an empty or mismatched API key", async () => {
    await expect(generateApiKeyHash(async () => "")).rejects.toThrow("API key is required");

    const values = ["api-key", "other-api-key"];
    await expect(generateApiKeyHash(async () => values.shift() ?? "")).rejects.toThrow(
      "API key confirmation does not match",
    );
  });
});
