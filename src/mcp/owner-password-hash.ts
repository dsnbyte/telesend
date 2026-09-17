import { AppError } from "../core/errors.ts";

export type ReadPassword = (prompt: string) => Promise<string>;

export function encodeOwnerPasswordHash(hash: string): string {
  return Buffer.from(hash).toString("base64url");
}

export function decodeOwnerPasswordHash(value: string): string | null {
  if (value.startsWith("$")) return value;
  if (!/^[A-Za-z0-9_-]+$/.test(value)) return null;

  const hash = Buffer.from(value, "base64url").toString();
  if (Buffer.from(hash).toString("base64url") !== value) return null;
  return hash;
}

export async function generateOwnerPasswordHash(readPassword: ReadPassword): Promise<string> {
  const password = await readPassword("Owner password: ");
  if (!password) throw new AppError("validation", "Owner password is required");

  const confirmation = await readPassword("Confirm owner password: ");
  if (password !== confirmation) {
    throw new AppError("validation", "Owner password confirmation does not match");
  }

  return Bun.password.hash(password);
}
