import { afterEach, describe, expect, test } from "bun:test";
import { mkdir, mkdtemp, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ensurePrivateDirectory, resolveTelesendPaths } from "../src/core/paths.ts";

const created: string[] = [];

afterEach(async () => {
  for (const path of created.splice(0)) {
    await Bun.$`rm -rf ${path}`.quiet();
  }
});

describe("resolveTelesendPaths", () => {
  test("uses XDG roots on Linux", () => {
    const paths = resolveTelesendPaths(
      { HOME: "/home/me", XDG_CONFIG_HOME: "/cfg", XDG_DATA_HOME: "/data" },
      "linux",
    );
    expect(paths.configFile).toBe("/cfg/telesend/config.toml");
    expect(paths.databaseFile).toBe("/data/telesend/telesend.db");
  });

  test("falls back to home on Linux", () => {
    const paths = resolveTelesendPaths({ HOME: "/home/me" }, "linux");
    expect(paths.configDir).toBe("/home/me/.config/telesend");
    expect(paths.dataDir).toBe("/home/me/.local/share/telesend");
  });

  test("uses native macOS roots", () => {
    const paths = resolveTelesendPaths({ HOME: "/Users/me" }, "darwin");
    expect(paths.dataDir).toBe("/Users/me/Library/Application Support/telesend");
  });

  test("uses Windows application roots", () => {
    const paths = resolveTelesendPaths(
      { APPDATA: "C:\\Roaming", LOCALAPPDATA: "C:\\Local" },
      "win32",
    );
    expect(paths.configDir).toContain("Roaming");
    expect(paths.dataDir).toContain("Local");
  });
});

test("ensurePrivateDirectory enforces owner-only permissions", async () => {
  const root = await mkdtemp(join(tmpdir(), "telesend-paths-"));
  created.push(root);
  const directory = join(root, "nested");
  await mkdir(directory, { mode: 0o777 });
  await ensurePrivateDirectory(directory);
  if (process.platform !== "win32") {
    expect((await stat(directory)).mode & 0o777).toBe(0o700);
  }
});
