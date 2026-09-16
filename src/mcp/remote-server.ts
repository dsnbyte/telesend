import {
  type AuthInfo,
  createMcpHandler,
  type McpHttpHandler,
  requireBearerAuth,
} from "@modelcontextprotocol/server";
import type { Application } from "../app.ts";
import { redactSecrets } from "../core/errors.ts";
import { TelesendOAuth } from "./oauth.ts";
import { OAuthRepository } from "./oauth-repository.ts";
import { loadRemoteMcpConfig, type RemoteMcpConfig } from "./remote-config.ts";
import { createRemoteMcpServer } from "./server.ts";

const MAX_MCP_REQUEST_BYTES = 1024 * 1024;

export interface RemoteMcpOptions {
  hostname?: string;
  port?: number;
}

export interface RemoteMcpServer {
  readonly url: URL;
  stop(closeActiveConnections?: boolean): void | Promise<void>;
}

interface RemoteMcpRuntime {
  config?: RemoteMcpConfig;
  log?: (message: string) => void;
  serve?: (options: {
    hostname?: string;
    port?: number;
    fetch(request: Request): Response | Promise<Response>;
  }) => RemoteMcpServer;
}

export function startRemoteMcpServer(
  app: Application,
  options: RemoteMcpOptions,
  runtime: RemoteMcpRuntime = {},
): RemoteMcpServer {
  const config = runtime.config ?? loadRemoteMcpConfig();
  const oauth = new TelesendOAuth(new OAuthRepository(app.database), config);
  const handler = createRemoteMcpHandler(app, oauth, config);
  const server = (runtime.serve ?? ((value) => Bun.serve(value)))({
    hostname: options.hostname ?? "127.0.0.1",
    ...(options.port === undefined ? {} : { port: options.port }),
    fetch: handler.fetch,
  });
  (runtime.log ?? console.log)(
    `Telesend remote MCP listening on ${server.url.toString()} (public ${new URL("/mcp", config.publicUrl).href})`,
  );
  return server;
}

export function createRemoteMcpHandler(
  app: Application,
  oauth: TelesendOAuth,
  config: RemoteMcpConfig,
): McpHttpHandler {
  const mcp = createMcpHandler(
    ({ authInfo }) => createRemoteMcpServer(app, authInfo?.scopes ?? []),
    { onerror: () => undefined },
  );
  const gate = requireBearerAuth({
    verifier: oauth,
    requiredScopes: ["mcp:read"],
    resourceMetadataUrl: new URL("/.well-known/oauth-protected-resource/mcp", config.publicUrl)
      .href,
  });

  return {
    ...mcp,
    fetch: async (request: Request): Promise<Response> => {
      try {
        const origin = request.headers.get("origin");
        if (origin && origin !== config.publicUrl.origin) {
          return safeJson("forbidden", "Cross-origin requests are not allowed", 403);
        }
        const length = Number(request.headers.get("content-length") ?? 0);
        if (Number.isFinite(length) && length > MAX_MCP_REQUEST_BYTES) {
          return safeJson("request_too_large", "Request body is too large", 413);
        }
        if (request.method === "POST") {
          const body = await request.clone().arrayBuffer();
          if (body.byteLength > MAX_MCP_REQUEST_BYTES) {
            return safeJson("request_too_large", "Request body is too large", 413);
          }
        }

        const oauthResponse = await oauth.handle(request);
        if (oauthResponse) return oauthResponse;

        const url = new URL(request.url);
        if (url.pathname !== "/mcp") return safeJson("not_found", "Route not found", 404);
        if (!new Set(["GET", "POST", "DELETE"]).has(request.method)) {
          return safeJson("method_not_allowed", "Method not allowed", 405, {
            allow: "GET, POST, DELETE",
          });
        }

        const auth: AuthInfo | Response = await gate(request);
        if (auth instanceof Response) return auth;
        return mcp.fetch(request, { authInfo: auth });
      } catch (error) {
        const message = redactSecrets(error, [config.ownerPasswordHash]);
        return safeJson("internal", message || "Remote MCP request failed", 500);
      }
    },
  };
}

function safeJson(
  code: string,
  message: string,
  status: number,
  headers: Record<string, string> = {},
): Response {
  return Response.json(
    { error: { code, message } },
    { status, headers: { "cache-control": "no-store", ...headers } },
  );
}
