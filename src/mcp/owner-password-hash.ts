import { AppError } from "../core/errors.ts";

export type ReadPassword = (prompt: string) => Promise<string>;

export async function generateOwnerPasswordHash(readPassword: ReadPassword): Promise<string> {
  const password = await readPassword("Owner password: ");
  if (!password) throw new AppError("validation", "Owner password is required");

  const confirmation = await readPassword("Confirm owner password: ");
  if (password !== confirmation) {
    throw new AppError("validation", "Owner password confirmation does not match");
  }

  return Bun.password.hash(password);
}
