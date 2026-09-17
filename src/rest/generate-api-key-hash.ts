#!/usr/bin/env bun

import { createInterface } from "node:readline/promises";
import { Writable } from "node:stream";
import { redactSecrets } from "../core/errors.ts";
import { encodeOwnerPasswordHash } from "../mcp/owner-password-hash.ts";
import { generateApiKeyHash } from "./api-key-hash.ts";

const output = new Writable({ write: (_chunk, _encoding, callback) => callback() });
const terminal = createInterface({ input: process.stdin, output, terminal: true });

try {
  const hash = await generateApiKeyHash(async (prompt) => {
    process.stderr.write(prompt);
    const value = await terminal.question("");
    process.stderr.write("\n");
    return value;
  });
  process.stdout.write(`TELESEND_API_KEY_HASH=${encodeOwnerPasswordHash(hash)}\n`);
} catch (error) {
  process.stderr.write(`Error: ${redactSecrets(error)}\n`);
  process.exitCode = 1;
} finally {
  terminal.close();
}
