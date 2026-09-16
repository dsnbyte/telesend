import { existsSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { TOML } from "bun";
import { AppError } from "../core/errors.ts";
import { type PathEnvironment, resolveTelesendPaths } from "../core/paths.ts";

export interface McpFileConfig {
  allowPaths: string[];
  include: string[];
  exclude: string[];
}

export interface LoadedMcpConfig {
  configPath: string;
  files: McpFileConfig;
}

export async function loadMcpConfig(
  options: { configPath?: string; env?: PathEnvironment | NodeJS.ProcessEnv; home?: string } = {},
): Promise<LoadedMcpConfig> {
  const env = options.env ?? process.env;
  const home = options.home ?? env.HOME;
  const defaultPath = resolveTelesendPaths(env).configFile;
  const explicit = options.configPath !== undefined;
  const configPath = expandHome(options.configPath ?? defaultPath, home);

  if (!existsSync(configPath)) {
    if (explicit)
      throw new AppError("configuration", `MCP config file was not found: ${configPath}`);
    return { configPath, files: { allowPaths: [], include: [], exclude: [] } };
  }

  let parsed: unknown;
  try {
    parsed = TOML.parse(await Bun.file(configPath).text());
  } catch (error) {
    throw new AppError("configuration", `Invalid MCP TOML config: ${String(error)}`);
  }
  const root = record(parsed, "config");
  const mcp = root.mcp === undefined ? {} : record(root.mcp, "mcp");
  const files = mcp.files === undefined ? {} : record(mcp.files, "mcp.files");
  const allowPaths = stringArray(files.allow_paths, "mcp.files.allow_paths").map((path) => {
    const expanded = expandHome(path, home);
    if (!isAbsolute(expanded)) {
      throw new AppError("configuration", "mcp.files.allow_paths entries must be absolute");
    }
    return expanded;
  });
  return {
    configPath,
    files: {
      allowPaths,
      include: stringArray(files.include, "mcp.files.include"),
      exclude: stringArray(files.exclude, "mcp.files.exclude"),
    },
  };
}

function expandHome(path: string, home?: string): string {
  if (path === "~" || path.startsWith("~/")) {
    if (!home) throw new AppError("configuration", "HOME is required to expand MCP paths");
    return path === "~" ? home : join(home, path.slice(2));
  }
  return path;
}

function record(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new AppError("configuration", `${name} must be a TOML table`);
  }
  return value as Record<string, unknown>;
}

function stringArray(value: unknown, name: string): string[] {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string" || !item)) {
    throw new AppError("configuration", `${name} must be an array of non-empty strings`);
  }
  return value as string[];
}
