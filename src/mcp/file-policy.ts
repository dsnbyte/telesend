import { constants } from "node:fs";
import { access, realpath, stat } from "node:fs/promises";
import { isAbsolute, relative, resolve, sep } from "node:path";
import { Glob } from "bun";
import { AppError } from "../core/errors.ts";

export const PROTECTED_EXCLUSIONS = [
  "**/.env",
  "**/.env.*",
  "**/*.{pem,key,p12,pfx}",
  "**/.git",
  "**/.git/**",
] as const;

export const DEFAULT_EXCLUSIONS = [
  "**/*.{ts,tsx,js,jsx,mjs,cjs,mts,cts}",
  "**/*.{py,go,rs,java,kt,kts,rb,php,c,h,cpp,hpp,cs,swift,scala}",
  "**/*.{sh,bash,zsh,fish,ps1,sql}",
  "**/*.{toml,yaml,yml,ini,conf}",
  "**/package.json",
  "**/tsconfig*.json",
  "**/jsconfig*.json",
  "**/bun.lock",
  "**/bun.lockb",
  "**/src/**",
  "**/source/**",
  "**/config/**",
  "**/configs/**",
  "**/assets/**",
  "**/node_modules/**",
  "**/vendor/**",
] as const;

export interface FilePolicyOptions {
  cwd: string;
  configuredRoots: string[];
  cliRoots: string[];
  include: string[];
  exclude: string[];
  protectedPaths: string[];
}

export class FilePolicy {
  private constructor(
    private readonly roots: string[],
    private readonly includes: Glob[],
    private readonly excludes: Glob[],
    private readonly protectedPaths: Set<string>,
    private readonly protectedPatterns = PROTECTED_EXCLUSIONS.map((pattern) => new Glob(pattern)),
    private readonly defaultPatterns = DEFAULT_EXCLUSIONS.map((pattern) => new Glob(pattern)),
  ) {}

  static async create(options: FilePolicyOptions): Promise<FilePolicy> {
    const rawRoots = [
      options.cwd,
      ...options.configuredRoots,
      ...options.cliRoots.map((path) => (isAbsolute(path) ? path : resolve(options.cwd, path))),
    ];
    const roots = [...new Set(await Promise.all(rawRoots.map((path) => canonicalDirectory(path))))];
    const protectedPaths = new Set(
      await Promise.all(
        options.protectedPaths.map(async (path) => {
          try {
            return await realpath(path);
          } catch {
            return resolve(path);
          }
        }),
      ),
    );
    return new FilePolicy(
      roots,
      options.include.map((pattern) => new Glob(pattern)),
      options.exclude.map((pattern) => new Glob(pattern)),
      protectedPaths,
    );
  }

  async authorize(path: string): Promise<string> {
    let canonical: string;
    try {
      canonical = await realpath(isAbsolute(path) ? path : resolve(path));
    } catch {
      throw new AppError("filesystem_policy", `Local file does not exist: ${path}`);
    }
    const info = await stat(canonical);
    if (!info.isFile())
      throw new AppError("filesystem_policy", `Local path is not a regular file: ${path}`);
    try {
      await access(canonical, constants.R_OK);
    } catch {
      throw new AppError("filesystem_policy", `Local file is not readable: ${path}`);
    }

    const root = this.roots.find((candidate) => inside(candidate, canonical));
    if (!root)
      throw new AppError("filesystem_policy", "Local file is outside the allowed MCP paths");
    const relativePath = normalize(relative(root, canonical));
    if (this.protectedPaths.has(canonical) || matches(this.protectedPatterns, relativePath)) {
      throw new AppError("filesystem_policy", "Local file is protected by MCP policy");
    }
    if (matches(this.excludes, relativePath)) {
      throw new AppError("filesystem_policy", "Local file matches an MCP exclude pattern");
    }
    if (matches(this.defaultPatterns, relativePath) && !matches(this.includes, relativePath)) {
      throw new AppError("filesystem_policy", "Local file matches a default MCP exclusion");
    }
    return canonical;
  }
}

function inside(root: string, path: string): boolean {
  const child = relative(root, path);
  return child === "" || (!child.startsWith(`..${sep}`) && child !== ".." && !isAbsolute(child));
}

function normalize(path: string): string {
  return path.split(sep).join("/");
}

function matches(patterns: readonly Glob[], path: string): boolean {
  return patterns.some((pattern) => pattern.match(path));
}

async function canonicalDirectory(path: string): Promise<string> {
  try {
    const canonical = await realpath(path);
    if (!(await stat(canonical)).isDirectory()) throw new Error("not a directory");
    return canonical;
  } catch {
    throw new AppError("configuration", `MCP allowed path is not a directory: ${path}`);
  }
}
