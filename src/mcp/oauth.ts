import { createHash, randomBytes, timingSafeEqual } from "node:crypto";
import {
  type AuthInfo,
  OAuthError,
  OAuthErrorCode,
  type OAuthTokenVerifier,
} from "@modelcontextprotocol/server";
import { ICON_PNG_BUFFER } from "./icon.ts";
import type { OAuthRepository } from "./oauth-repository.ts";
import type { RemoteMcpConfig } from "./remote-config.ts";

const ACCESS_TOKEN_TTL = 60 * 60;
const AUTHORIZATION_CODE_TTL = 5 * 60;
const REFRESH_TOKEN_TTL = 30 * 24 * 60 * 60;
const SCOPES = ["mcp:read", "mcp:send"] as const;

interface OAuthRuntime {
  now?: () => number;
  randomToken?: () => string;
  verifyPassword?: (password: string, hash: string) => Promise<boolean>;
}

export class TelesendOAuth implements OAuthTokenVerifier {
  private readonly now: () => number;
  private readonly randomToken: () => string;
  private readonly verifyPassword: (password: string, hash: string) => Promise<boolean>;

  constructor(
    private readonly repository: OAuthRepository,
    private readonly config: RemoteMcpConfig,
    runtime: OAuthRuntime = {},
  ) {
    this.now = runtime.now ?? (() => Math.floor(Date.now() / 1000));
    this.randomToken = runtime.randomToken ?? (() => randomBytes(32).toString("base64url"));
    this.verifyPassword = runtime.verifyPassword ?? Bun.password.verify;
  }

  async handle(request: Request): Promise<Response | null> {
    const { pathname } = new URL(request.url);
    if (
      (request.method === "GET" || request.method === "HEAD") &&
      (pathname === "/icon.png" ||
        pathname === "/favicon.ico" ||
        pathname === "/apple-touch-icon.png" ||
        pathname === "/telesend-icon.png")
    ) {
      return new Response(request.method === "HEAD" ? null : ICON_PNG_BUFFER, {
        headers: {
          "content-type": "image/png",
          "cache-control": "public, max-age=86400, immutable",
          "content-length": ICON_PNG_BUFFER.byteLength.toString(),
        },
      });
    }
    if (
      request.method === "GET" &&
      (pathname === "/.well-known/oauth-protected-resource" ||
        pathname === "/.well-known/oauth-protected-resource/mcp")
    ) {
      return json({
        resource: this.endpoint("/mcp"),
        authorization_servers: [this.config.publicUrl.origin],
        scopes_supported: SCOPES,
        logo_uri: this.endpoint("/icon.png"),
        icon_uri: this.endpoint("/icon.png"),
      });
    }
    if (
      request.method === "GET" &&
      (pathname === "/.well-known/oauth-authorization-server" ||
        pathname === "/.well-known/openid-configuration")
    ) {
      return json({
        issuer: this.config.publicUrl.origin,
        authorization_endpoint: this.endpoint("/authorize"),
        token_endpoint: this.endpoint("/token"),
        registration_endpoint: this.endpoint("/register"),
        revocation_endpoint: this.endpoint("/revoke"),
        response_types_supported: ["code"],
        grant_types_supported: ["authorization_code", "refresh_token"],
        token_endpoint_auth_methods_supported: ["none"],
        code_challenge_methods_supported: ["S256"],
        scopes_supported: SCOPES,
        logo_uri: this.endpoint("/icon.png"),
        icon_uri: this.endpoint("/icon.png"),
      });
    }
    if (pathname === "/register" && request.method === "POST") return this.register(request);
    if (pathname === "/authorize" && (request.method === "GET" || request.method === "POST")) {
      return this.authorize(request);
    }
    if (pathname === "/token" && request.method === "POST") return this.token(request);
    if (pathname === "/revoke" && request.method === "POST") return this.revoke(request);
    return null;
  }

  async verifyAccessToken(token: string): Promise<AuthInfo> {
    const record = this.repository.findAccessToken(digest(token));
    const now = this.now();
    if (!record || record.revokedAt !== null || record.expiresAt <= now) {
      throw new OAuthError(OAuthErrorCode.InvalidToken, "Access token is invalid or expired");
    }
    return {
      token,
      clientId: record.clientId,
      scopes: record.scopes,
      expiresAt: record.expiresAt,
      resource: new URL(this.endpoint("/mcp")),
      extra: { grantId: record.grantId },
    };
  }

  private async register(request: Request): Promise<Response> {
    const body = await readJson(request);
    const redirectUris = body.redirect_uris;
    if (
      !Array.isArray(redirectUris) ||
      redirectUris.length === 0 ||
      redirectUris.length > 10 ||
      !redirectUris.every((value) => typeof value === "string" && validRedirectUri(value))
    ) {
      return oauthError("invalid_redirect_uri", "redirect_uris must contain valid callback URLs");
    }
    if (
      body.token_endpoint_auth_method !== undefined &&
      body.token_endpoint_auth_method !== "none"
    ) {
      return oauthError("invalid_client_metadata", "Only public PKCE clients are supported");
    }
    const name =
      typeof body.client_name === "string" && body.client_name.trim()
        ? body.client_name.trim().slice(0, 120)
        : "MCP client";
    const client = this.repository.registerClient(
      {
        id: `client_${this.randomToken()}`,
        name,
        redirectUris: [...new Set(redirectUris as string[])],
      },
      this.now(),
    );
    return json(
      {
        client_id: client.id,
        client_name: client.name,
        redirect_uris: client.redirectUris,
        token_endpoint_auth_method: "none",
      },
      201,
    );
  }

  private async authorize(request: Request): Promise<Response> {
    const params =
      request.method === "GET"
        ? new URL(request.url).searchParams
        : new URLSearchParams(await request.text());
    const validated = this.validateAuthorizationRequest(params);
    if (validated instanceof Response) return validated;

    if (request.method === "GET") {
      return new Response(consentPage(validated), {
        headers: {
          "cache-control": "no-store",
          "content-type": "text/html; charset=utf-8",
          "x-content-type-options": "nosniff",
          "x-frame-options": "DENY",
        },
      });
    }

    const password = params.get("password") ?? "";
    if (!(await this.verifyPassword(password, this.config.ownerPasswordHash))) {
      return oauthError("access_denied", "Owner authentication failed", 401);
    }

    const grantId = `grant_${this.randomToken()}`;
    this.repository.createGrant(
      { id: grantId, clientId: validated.clientId, scopes: validated.scopes },
      this.now(),
    );
    const code = `code_${this.randomToken()}`;
    this.repository.storeAuthorizationCode({
      codeChallenge: validated.codeChallenge,
      digest: digest(code),
      expiresAt: this.now() + AUTHORIZATION_CODE_TTL,
      grantId,
      redirectUri: validated.redirectUri,
    });
    const redirect = new URL(validated.redirectUri);
    redirect.searchParams.set("code", code);
    if (validated.state) redirect.searchParams.set("state", validated.state);
    return Response.redirect(redirect, 302);
  }

  private validateAuthorizationRequest(params: URLSearchParams): AuthorizationRequest | Response {
    const clientId = params.get("client_id") ?? "";
    const client = this.repository.findClient(clientId);
    const redirectUri = params.get("redirect_uri") ?? "";
    if (!client?.redirectUris.includes(redirectUri)) {
      return oauthError("invalid_request", "Client or redirect URI is invalid");
    }
    if (params.get("response_type") !== "code") {
      return oauthError("unsupported_response_type", "response_type must be code");
    }
    const codeChallenge = params.get("code_challenge") ?? "";
    if (params.get("code_challenge_method") !== "S256" || !isBase64Url(codeChallenge, 43, 128)) {
      return oauthError("invalid_request", "PKCE S256 is required");
    }
    const resource = params.get("resource");
    if (resource && resource !== this.endpoint("/mcp")) {
      return oauthError("invalid_target", "OAuth resource is invalid");
    }
    const scopes = parseScopes(params.get("scope"));
    if (!scopes) return oauthError("invalid_scope", "Requested OAuth scope is invalid");
    return {
      clientId,
      clientName: client.name,
      codeChallenge,
      redirectUri,
      scopes,
      state: params.get("state") ?? "",
    };
  }

  private async token(request: Request): Promise<Response> {
    if (!isFormRequest(request)) {
      return oauthError("invalid_request", "Token requests must use form encoding", 415);
    }
    const params = new URLSearchParams(await request.text());
    const grantType = params.get("grant_type");
    if (grantType === "authorization_code") return this.exchangeCode(params);
    if (grantType === "refresh_token") return this.refresh(params);
    return oauthError("unsupported_grant_type", "Unsupported grant type");
  }

  private exchangeCode(params: URLSearchParams): Response {
    const code = params.get("code") ?? "";
    const record = this.repository.consumeAuthorizationCode(digest(code), this.now());
    const verifier = params.get("code_verifier") ?? "";
    if (
      !record ||
      record.clientId !== params.get("client_id") ||
      record.redirectUri !== params.get("redirect_uri") ||
      !validPkce(verifier, record.codeChallenge)
    ) {
      return oauthError("invalid_grant", "Authorization code is invalid or expired");
    }
    return this.issueTokens(record.grantId, record.scopes);
  }

  private refresh(params: URLSearchParams): Response {
    const refreshToken = params.get("refresh_token") ?? "";
    const accessToken = `access_${this.randomToken()}`;
    const nextRefreshToken = `refresh_${this.randomToken()}`;
    const now = this.now();
    const record = this.repository.rotateRefreshToken(
      digest(refreshToken),
      params.get("client_id") ?? "",
      {
        accessDigest: digest(accessToken),
        accessExpiresAt: now + ACCESS_TOKEN_TTL,
        refreshDigest: digest(nextRefreshToken),
        refreshExpiresAt: now + REFRESH_TOKEN_TTL,
      },
      now,
    );
    if (!record) {
      return oauthError("invalid_grant", "Refresh token is invalid or expired");
    }
    return tokenResponse(accessToken, nextRefreshToken, record.scopes);
  }

  private issueTokens(grantId: string, scopes: string[]): Response {
    const accessToken = `access_${this.randomToken()}`;
    const refreshToken = `refresh_${this.randomToken()}`;
    const now = this.now();
    this.repository.storeTokens({
      accessDigest: digest(accessToken),
      accessExpiresAt: now + ACCESS_TOKEN_TTL,
      grantId,
      refreshDigest: digest(refreshToken),
      refreshExpiresAt: now + REFRESH_TOKEN_TTL,
    });
    return tokenResponse(accessToken, refreshToken, scopes);
  }

  private async revoke(request: Request): Promise<Response> {
    if (!isFormRequest(request)) {
      return oauthError("invalid_request", "Revocation requests must use form encoding", 415);
    }
    const token = new URLSearchParams(await request.text()).get("token") ?? "";
    if (token) this.repository.revokeGrantByToken(digest(token), this.now());
    return new Response(null, { status: 200, headers: { "cache-control": "no-store" } });
  }

  private endpoint(path: string): string {
    return new URL(path, this.config.publicUrl).href;
  }
}

interface AuthorizationRequest {
  clientId: string;
  clientName: string;
  codeChallenge: string;
  redirectUri: string;
  scopes: string[];
  state: string;
}

function consentPage(request: AuthorizationRequest): string {
  const hidden = {
    response_type: "code",
    client_id: request.clientId,
    redirect_uri: request.redirectUri,
    scope: request.scopes.join(" "),
    state: request.state,
    code_challenge: request.codeChallenge,
    code_challenge_method: "S256",
  };
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authorize Telesend</title>
    <link rel="icon" type="image/png" href="/icon.png">
    <link rel="apple-touch-icon" href="/icon.png">
    <style>
      :root {
        color-scheme: light;
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }
      * { box-sizing: border-box; }
      body {
        min-height: 100vh;
        margin: 0;
        display: grid;
        place-items: center;
        padding: 1.5rem 1.5rem 7rem;
        background: #f6f8fc;
        color: #172033;
      }
      main {
        width: min(100%, 26rem);
        padding: 2rem;
        background: #fff;
        border: 1px solid #dfe5ef;
        border-radius: 1rem;
        box-shadow: 0 1rem 2.5rem rgb(30 47 80 / 10%);
      }
      .app-icon-wrap {
        display: flex;
        justify-content: center;
        align-items: center;
        margin-bottom: 1.25rem;
      }
      .app-icon {
        width: 5.25rem;
        height: 5.25rem;
        border-radius: 1.125rem;
        object-fit: contain;
        box-shadow: 0 0.5rem 1.25rem rgb(30 47 80 / 12%);
        border: 1px solid #dfe5ef;
      }
      h1 {
        margin: 0;
        font-size: 1.375rem;
        line-height: 1.35;
        letter-spacing: -0.02em;
        text-align: center;
      }
      p {
        margin: 0.625rem 0 1.5rem;
        color: #5a667c;
        font-size: 0.9375rem;
        line-height: 1.5;
        text-align: center;
      }
      form { display: grid; gap: 1rem; }
      label {
        display: grid;
        gap: 0.5rem;
        color: #35425a;
        font-size: 0.875rem;
        font-weight: 600;
      }
      input {
        width: 100%;
        min-height: 2.75rem;
        padding: 0.625rem 0.75rem;
        border: 1px solid #c9d2e1;
        border-radius: 0.625rem;
        color: inherit;
        font: inherit;
        outline: none;
        transition: border-color 150ms ease, box-shadow 150ms ease;
      }
      input:focus {
        border-color: #2563eb;
        box-shadow: 0 0 0 3px rgb(37 99 235 / 15%);
      }
      button {
        min-height: 2.75rem;
        border: 0;
        border-radius: 0.625rem;
        background: #408EC7;
        color: #fff;
        cursor: pointer;
        font: inherit;
        font-weight: 650;
        transition: background 150ms ease, transform 150ms ease;
      }
      button:hover { background: #1d4ed8; }
      button:active { transform: translateY(1px); }
      button:focus-visible { outline: 3px solid rgb(37 99 235 / 30%); outline-offset: 3px; }
      @media (max-width: 30rem) {
        body { padding: 1.5rem 1rem 3rem; }
        main { padding: 1.5rem; }
      }
    </style>
  </head>
  <body>
    <main>
      <div class="app-icon-wrap"><img src="/icon.png" alt="Telesend" class="app-icon" width="84" height="84"></div>
      <h1>Authorize ${escapeHtml(request.clientName)}</h1>
      <p>Requested access: ${escapeHtml(request.scopes.join(", "))}</p>
      <form method="post" action="/authorize">${Object.entries(hidden)
        .map(
          ([name, value]) =>
            `<input type="hidden" name="${escapeHtml(name)}" value="${escapeHtml(value)}">`,
        )
        .join(
          "",
        )}<label>Owner password <input type="password" name="password" required autocomplete="current-password"></label><button type="submit">Authorize</button></form>
    </main>
  </body>
</html>`;
}

function tokenResponse(accessToken: string, refreshToken: string, scopes: string[]): Response {
  return json(
    {
      access_token: accessToken,
      token_type: "Bearer",
      expires_in: ACCESS_TOKEN_TTL,
      refresh_token: refreshToken,
      scope: scopes.join(" "),
    },
    200,
    { "cache-control": "no-store", pragma: "no-cache" },
  );
}

function parseScopes(value: string | null): string[] | null {
  const values = value?.trim() ? [...new Set(value.trim().split(/\s+/))] : [...SCOPES];
  return values.length > 0 &&
    values.every((scope) => SCOPES.includes(scope as (typeof SCOPES)[number]))
    ? values
    : null;
}

function validPkce(verifier: string, expected: string): boolean {
  if (!isBase64Url(verifier, 43, 128)) return false;
  const actual = createHash("sha256").update(verifier).digest("base64url");
  return safeEqual(actual, expected);
}

function safeEqual(left: string, right: string): boolean {
  const first = Buffer.from(left);
  const second = Buffer.from(right);
  return first.length === second.length && timingSafeEqual(first, second);
}

function digest(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

function isBase64Url(value: string, minimum: number, maximum: number): boolean {
  return value.length >= minimum && value.length <= maximum && /^[A-Za-z0-9_-]+$/.test(value);
}

function validRedirectUri(value: string): boolean {
  try {
    const url = new URL(value);
    return (
      (url.protocol === "https:" ||
        (url.protocol === "http:" &&
          (url.hostname === "127.0.0.1" || url.hostname === "localhost"))) &&
      !url.username &&
      !url.password &&
      !url.hash
    );
  } catch {
    return false;
  }
}

function isFormRequest(request: Request): boolean {
  return (
    request.headers.get("content-type")?.split(";", 1)[0]?.trim() ===
    "application/x-www-form-urlencoded"
  );
}

async function readJson(request: Request): Promise<Record<string, unknown>> {
  try {
    const value = await request.json();
    return value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

function oauthError(error: string, description: string, status = 400): Response {
  return json({ error, error_description: description }, status, {
    "cache-control": "no-store",
    pragma: "no-cache",
  });
}

function json(value: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return Response.json(value, { status, headers });
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => {
    const replacements: Record<string, string> = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return replacements[character] as string;
  });
}

export const oauthTesting = { digest };
