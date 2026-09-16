import { afterEach, describe, expect, test } from "bun:test";
import { mkdir, mkdtemp, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { loadMcpConfig } from "../src/mcp/config.ts";
import { DEFAULT_EXCLUSIONS, FilePolicy, PROTECTED_EXCLUSIONS } from "../src/mcp/file-policy.ts";

const roots: string[] = [];
afterEach(async () => {
  for (const root of roots.splice(0)) await Bun.$`rm -rf ${root}`.quiet();
});

async function root(): Promise<string> {
  const path = await mkdtemp(join(tmpdir(), "telesend-policy-"));
  roots.push(path);
  return path;
}

describe("MCP TOML config", () => {
  test("loads the shipped example with the production loader", async () => {
    const home = await root();
    const config = await loadMcpConfig({
      configPath: join(import.meta.dir, "..", "examples", "config.toml"),
      env: { HOME: home },
      home,
    });
    expect(config.files.allowPaths).toEqual([join(home, "Documents", "telesend")]);
    expect(config.files.include).toEqual(["generated/**/*.json"]);
    expect(config.files.exclude).toEqual(["**/private/**", "**/*-secret.*"]);
  });

  test("uses safe defaults when the default file is missing", async () => {
    const home = await root();
    const config = await loadMcpConfig({ env: { HOME: home }, home });
    expect(config.files).toEqual({ allowPaths: [], include: [], exclude: [] });
  });

  test("loads and validates a TOML policy", async () => {
    const home = await root();
    const allowed = join(home, "reports");
    await mkdir(allowed);
    const path = join(home, "policy.toml");
    await Bun.write(
      path,
      `[mcp.files]\nallow_paths = ["${allowed}"]\ninclude = ["reports/**/*.json"]\nexclude = ["**/private/**"]\n`,
    );
    expect((await loadMcpConfig({ configPath: path, env: { HOME: home }, home })).files).toEqual({
      allowPaths: [allowed],
      include: ["reports/**/*.json"],
      exclude: ["**/private/**"],
    });
  });

  test("fails closed for explicit missing, invalid TOML, and relative roots", async () => {
    const home = await root();
    await expect(
      loadMcpConfig({ configPath: join(home, "missing.toml"), env: { HOME: home } }),
    ).rejects.toThrow("was not found");
    const invalid = join(home, "invalid.toml");
    await Bun.write(invalid, "invalid = =");
    await expect(loadMcpConfig({ configPath: invalid, env: { HOME: home } })).rejects.toThrow(
      "Invalid MCP TOML",
    );
    const relative = join(home, "relative.toml");
    await Bun.write(relative, '[mcp.files]\nallow_paths = ["reports"]\n');
    await expect(loadMcpConfig({ configPath: relative, env: { HOME: home } })).rejects.toThrow(
      "must be absolute",
    );
  });
});

describe("MCP file policy", () => {
  test("defines protected and default application exclusions", () => {
    expect(PROTECTED_EXCLUSIONS).toContain("**/.env");
    expect(PROTECTED_EXCLUSIONS).toContain("**/*.{pem,key,p12,pfx}");
    expect(DEFAULT_EXCLUSIONS).toContain("**/*.{ts,tsx,js,jsx,mjs,cjs,mts,cts}");
    expect(DEFAULT_EXCLUSIONS).toContain("**/assets/**");
  });

  test("allows regular files under cwd and CLI/config roots", async () => {
    const cwd = await root();
    const configured = await root();
    const cli = join(cwd, "cli");
    await mkdir(cli);
    for (const path of [join(cwd, "a.pdf"), join(configured, "b.pdf"), join(cli, "c.pdf")]) {
      await Bun.write(path, "content");
    }
    const policy = await FilePolicy.create({
      cwd,
      configuredRoots: [configured],
      cliRoots: ["cli"],
      include: [],
      exclude: [],
      protectedPaths: [],
    });
    await expect(policy.authorize(join(cwd, "a.pdf"))).resolves.toBe(join(cwd, "a.pdf"));
    await expect(policy.authorize(join(configured, "b.pdf"))).resolves.toBe(
      join(configured, "b.pdf"),
    );
    await expect(policy.authorize(join(cli, "c.pdf"))).resolves.toBe(join(cli, "c.pdf"));
  });

  test("rejects outside paths, prefix confusion, directories, and symlink escapes", async () => {
    const allowed = await root();
    const outside = await root();
    const secret = join(outside, "secret.pdf");
    await Bun.write(secret, "secret");
    const link = join(allowed, "link.pdf");
    await symlink(secret, link);
    const policy = await FilePolicy.create({
      cwd: allowed,
      configuredRoots: [],
      cliRoots: [],
      include: [],
      exclude: [],
      protectedPaths: [],
    });
    await expect(policy.authorize(secret)).rejects.toThrow("outside");
    await expect(policy.authorize(link)).rejects.toThrow("outside");
    await expect(policy.authorize(allowed)).rejects.toThrow("regular file");
  });

  test("enforces protected, exclude, default, and include precedence", async () => {
    const allowed = await root();
    const paths = {
      env: join(allowed, ".env"),
      source: join(allowed, "report.ts"),
      included: join(allowed, "included.ts"),
      excluded: join(allowed, "excluded.ts"),
      safe: join(allowed, "safe.pdf"),
      database: join(allowed, "telesend.db"),
    };
    for (const path of Object.values(paths)) await Bun.write(path, "content");
    const policy = await FilePolicy.create({
      cwd: allowed,
      configuredRoots: [],
      cliRoots: [],
      include: ["included.ts", "excluded.ts", ".env"],
      exclude: ["excluded.ts"],
      protectedPaths: [paths.database],
    });
    await expect(policy.authorize(paths.env)).rejects.toThrow("protected");
    await expect(policy.authorize(paths.database)).rejects.toThrow("protected");
    await expect(policy.authorize(paths.source)).rejects.toThrow("default MCP exclusion");
    await expect(policy.authorize(paths.included)).resolves.toBe(paths.included);
    await expect(policy.authorize(paths.excluded)).rejects.toThrow("exclude pattern");
    await expect(policy.authorize(paths.safe)).resolves.toBe(paths.safe);
  });
});
