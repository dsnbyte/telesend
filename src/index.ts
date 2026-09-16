#!/usr/bin/env bun

import { createApplication } from "./app.ts";
import { runCli } from "./cli.ts";
import { resolveTelesendPaths } from "./core/paths.ts";
import { startMcpServer } from "./mcp/server.ts";
import { startRestServer } from "./rest/server.ts";

const paths = resolveTelesendPaths();
const app = await createApplication({ databasePath: paths.databaseFile });
const exitCode = await runCli(Bun.argv.slice(2), {
  aliases: app.aliases,
  bots: app.bots,
  delivery: app.delivery,
  startMcp: (options) => startMcpServer(app, options),
  startRest: async (options) => {
    startRestServer(app, options);
  },
});

if (!Bun.argv.slice(2).some((argument) => argument === "serve" || argument === "mcp")) {
  app.database.close(false);
}
process.exitCode = exitCode;
