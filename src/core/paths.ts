import { chmod, mkdir } from "node:fs/promises";
import { join } from "node:path";

export interface PathEnvironment {
  APPDATA?: string;
  HOME?: string;
  LOCALAPPDATA?: string;
  XDG_CONFIG_HOME?: string;
  XDG_DATA_HOME?: string;
}

export interface TelesendPaths {
  configDir: string;
  configFile: string;
  dataDir: string;
  databaseFile: string;
}

export function resolveTelesendPaths(
  env: PathEnvironment | NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
): TelesendPaths {
  const home = env.HOME;
  if (!home && platform !== "win32") {
    throw new Error("HOME is required when XDG paths are not configured");
  }

  let dataRoot: string;
  let configRoot: string;

  if (platform === "win32") {
    dataRoot = env.LOCALAPPDATA ?? join(env.HOME ?? "", "AppData", "Local");
    configRoot = env.APPDATA ?? join(env.HOME ?? "", "AppData", "Roaming");
  } else if (platform === "darwin") {
    dataRoot = env.XDG_DATA_HOME ?? join(home as string, "Library", "Application Support");
    configRoot = env.XDG_CONFIG_HOME ?? join(home as string, "Library", "Preferences");
  } else {
    dataRoot = env.XDG_DATA_HOME ?? join(home as string, ".local", "share");
    configRoot = env.XDG_CONFIG_HOME ?? join(home as string, ".config");
  }

  const dataDir = join(dataRoot, "telesend");
  const configDir = join(configRoot, "telesend");
  return {
    dataDir,
    configDir,
    databaseFile: join(dataDir, "telesend.db"),
    configFile: join(configDir, "config.toml"),
  };
}

export async function ensurePrivateDirectory(path: string): Promise<void> {
  await mkdir(path, { recursive: true, mode: 0o700 });
  if (process.platform !== "win32") {
    await chmod(path, 0o700);
  }
}
