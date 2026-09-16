import { AppError } from "../core/errors.ts";

export interface RemoteMcpConfig {
  ownerPasswordHash: string;
  publicUrl: URL;
}

export function loadRemoteMcpConfig(
  environment: Record<string, string | undefined> = process.env,
): RemoteMcpConfig {
  const publicValue = environment.TELESEND_MCP_PUBLIC_URL?.trim();
  if (!publicValue) {
    throw new AppError("configuration", "TELESEND_MCP_PUBLIC_URL is required");
  }
  let publicUrl: URL;
  try {
    publicUrl = new URL(publicValue);
  } catch {
    throw new AppError("configuration", "TELESEND_MCP_PUBLIC_URL must be a valid HTTPS origin");
  }
  if (
    publicUrl.protocol !== "https:" ||
    publicUrl.username ||
    publicUrl.password ||
    publicUrl.pathname !== "/" ||
    publicUrl.search ||
    publicUrl.hash
  ) {
    throw new AppError("configuration", "TELESEND_MCP_PUBLIC_URL must be an HTTPS origin");
  }

  const ownerPasswordHash = environment.TELESEND_MCP_OWNER_PASSWORD_HASH?.trim();
  if (ownerPasswordHash) {
    if (!/^\$(?:argon2|2[aby]\$)/.test(ownerPasswordHash)) {
      throw new AppError(
        "configuration",
        "TELESEND_MCP_OWNER_PASSWORD_HASH must contain a Bun-compatible password hash",
      );
    }
    return { ownerPasswordHash, publicUrl };
  }

  const ownerPassword = environment.TELESEND_MCP_OWNER_PASSWORD;
  if (!ownerPassword) {
    throw new AppError(
      "configuration",
      "TELESEND_MCP_OWNER_PASSWORD or TELESEND_MCP_OWNER_PASSWORD_HASH is required",
    );
  }
  return { ownerPasswordHash: Bun.password.hashSync(ownerPassword), publicUrl };
}
