import { AppError } from "../core/errors.ts";

export type ReadApiKey = (prompt: string) => Promise<string>;

export async function generateApiKeyHash(readApiKey: ReadApiKey): Promise<string> {
  const apiKey = await readApiKey("API key: ");
  if (!apiKey) throw new AppError("validation", "API key is required");

  const confirmation = await readApiKey("Confirm API key: ");
  if (apiKey !== confirmation) {
    throw new AppError("validation", "API key confirmation does not match");
  }

  return Bun.password.hash(apiKey);
}
