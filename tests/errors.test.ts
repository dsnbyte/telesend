import { expect, test } from "bun:test";
import { redactSecrets } from "../src/core/errors.ts";

test("redacts configured secrets", () => {
  expect(redactSecrets("key=top-secret", ["top-secret"])).toBe("key=[REDACTED]");
});

test("redacts Telegram tokens and token-bearing URLs", () => {
  const token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
  const output = redactSecrets(`failed ${token} https://api.telegram.org/bot${token}/sendMessage`);
  expect(output).not.toContain(token);
  expect(output).toContain("/bot[REDACTED]/sendMessage");
});
