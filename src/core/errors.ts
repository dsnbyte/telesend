export type ErrorCode =
  | "configuration"
  | "conflict"
  | "filesystem_policy"
  | "not_found"
  | "telegram"
  | "unauthorized"
  | "validation";

export class AppError extends Error {
  readonly code: ErrorCode;
  readonly details?: Record<string, unknown>;

  constructor(code: ErrorCode, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "AppError";
    this.code = code;
    if (details) this.details = details;
  }
}

const TELEGRAM_TOKEN = /\b\d{6,}:[A-Za-z0-9_-]{20,}\b/g;
const TELEGRAM_TOKEN_URL = /\/bot[^/\s]+\//g;

export function redactSecrets(value: unknown, secrets: readonly string[] = []): string {
  let text = value instanceof Error ? value.message : String(value);
  for (const secret of secrets) {
    if (secret) text = text.replaceAll(secret, "[REDACTED]");
  }
  return text.replace(TELEGRAM_TOKEN_URL, "/bot[REDACTED]/").replace(TELEGRAM_TOKEN, "[REDACTED]");
}

export function toAppError(error: unknown, secrets: readonly string[] = []): AppError {
  if (error instanceof AppError) {
    return new AppError(error.code, redactSecrets(error.message, secrets), error.details);
  }
  return new AppError("telegram", redactSecrets(error, secrets));
}
