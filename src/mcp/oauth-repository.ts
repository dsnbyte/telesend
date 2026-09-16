import type { Database } from "bun:sqlite";

export interface OAuthClient {
  id: string;
  name: string;
  redirectUris: string[];
}

export interface OAuthCredentialRecord {
  clientId: string;
  expiresAt: number;
  grantId: string;
  revokedAt: number | null;
  scopes: string[];
}

export interface OAuthAuthorizationCode extends OAuthCredentialRecord {
  codeChallenge: string;
  consumedAt: number | null;
  redirectUri: string;
}

type ClientRow = { id: string; name: string; redirect_uris: string };
type CredentialRow = {
  client_id: string;
  expires_at: number;
  grant_id: string;
  revoked_at: number | null;
  scopes: string;
};
type CodeRow = CredentialRow & {
  code_challenge: string;
  consumed_at: number | null;
  redirect_uri: string;
};

export class OAuthRepository {
  constructor(private readonly database: Database) {}

  registerClient(input: OAuthClient, now: number): OAuthClient {
    this.database
      .query("INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, ?, ?, ?)")
      .run(input.id, input.name, JSON.stringify(input.redirectUris), now);
    return input;
  }

  findClient(id: string): OAuthClient | null {
    const row = this.database
      .query("SELECT id, name, redirect_uris FROM oauth_clients WHERE id = ?")
      .get(id) as ClientRow | null;
    return row ? client(row) : null;
  }

  createGrant(input: { id: string; clientId: string; scopes: string[] }, now: number): void {
    this.database
      .query("INSERT INTO oauth_grants (id, client_id, scopes, created_at) VALUES (?, ?, ?, ?)")
      .run(input.id, input.clientId, input.scopes.join(" "), now);
  }

  storeAuthorizationCode(input: {
    codeChallenge: string;
    digest: string;
    expiresAt: number;
    grantId: string;
    redirectUri: string;
  }): void {
    this.database
      .query(
        `INSERT INTO oauth_authorization_codes
          (digest, grant_id, redirect_uri, code_challenge, expires_at)
         VALUES (?, ?, ?, ?, ?)`,
      )
      .run(input.digest, input.grantId, input.redirectUri, input.codeChallenge, input.expiresAt);
  }

  consumeAuthorizationCode(digest: string, now: number): OAuthAuthorizationCode | null {
    return this.database.transaction(() => {
      const row = this.database
        .query(
          `SELECT c.redirect_uri, c.code_challenge, c.expires_at, c.consumed_at,
                  g.id AS grant_id, g.client_id, g.scopes, g.revoked_at
             FROM oauth_authorization_codes c
             JOIN oauth_grants g ON g.id = c.grant_id
            WHERE c.digest = ?`,
        )
        .get(digest) as CodeRow | null;
      if (!row || row.consumed_at !== null || row.expires_at <= now || row.revoked_at !== null) {
        return null;
      }
      this.database
        .query("UPDATE oauth_authorization_codes SET consumed_at = ? WHERE digest = ?")
        .run(now, digest);
      return authorizationCode(row);
    })();
  }

  storeTokens(input: {
    accessDigest: string;
    accessExpiresAt: number;
    grantId: string;
    refreshDigest: string;
    refreshExpiresAt: number;
  }): void {
    this.database.transaction(() => {
      this.database
        .query("INSERT INTO oauth_access_tokens (digest, grant_id, expires_at) VALUES (?, ?, ?)")
        .run(input.accessDigest, input.grantId, input.accessExpiresAt);
      this.database
        .query("INSERT INTO oauth_refresh_tokens (digest, grant_id, expires_at) VALUES (?, ?, ?)")
        .run(input.refreshDigest, input.grantId, input.refreshExpiresAt);
    })();
  }

  findAccessToken(digest: string): OAuthCredentialRecord | null {
    const row = this.database
      .query(
        `SELECT t.expires_at, COALESCE(t.revoked_at, g.revoked_at) AS revoked_at,
                g.id AS grant_id, g.client_id, g.scopes
           FROM oauth_access_tokens t
           JOIN oauth_grants g ON g.id = t.grant_id
          WHERE t.digest = ?`,
      )
      .get(digest) as CredentialRow | null;
    return row ? credential(row) : null;
  }

  rotateRefreshToken(
    digest: string,
    clientId: string,
    replacement: {
      accessDigest: string;
      accessExpiresAt: number;
      refreshDigest: string;
      refreshExpiresAt: number;
    },
    now: number,
  ): OAuthCredentialRecord | null {
    return this.database.transaction(() => {
      const row = this.database
        .query(
          `SELECT t.expires_at, COALESCE(t.revoked_at, g.revoked_at) AS revoked_at,
                  g.id AS grant_id, g.client_id, g.scopes
             FROM oauth_refresh_tokens t
             JOIN oauth_grants g ON g.id = t.grant_id
            WHERE t.digest = ? AND g.client_id = ? AND t.replaced_by_digest IS NULL`,
        )
        .get(digest, clientId) as CredentialRow | null;
      if (!row || row.revoked_at !== null || row.expires_at <= now) return null;

      this.database
        .query(
          "UPDATE oauth_refresh_tokens SET revoked_at = ?, replaced_by_digest = ? WHERE digest = ?",
        )
        .run(now, replacement.refreshDigest, digest);
      this.storeTokens({ ...replacement, grantId: row.grant_id });
      return credential(row);
    })();
  }

  revokeGrantByToken(digest: string, now: number): boolean {
    return this.database.transaction(() => {
      const row = this.database
        .query(
          `SELECT grant_id FROM oauth_access_tokens WHERE digest = ?
           UNION SELECT grant_id FROM oauth_refresh_tokens WHERE digest = ?`,
        )
        .get(digest, digest) as { grant_id: string } | null;
      if (!row) return false;
      this.database
        .query("UPDATE oauth_grants SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?")
        .run(now, row.grant_id);
      return true;
    })();
  }
}

function client(row: ClientRow): OAuthClient {
  return { id: row.id, name: row.name, redirectUris: JSON.parse(row.redirect_uris) as string[] };
}

function credential(row: CredentialRow): OAuthCredentialRecord {
  return {
    clientId: row.client_id,
    expiresAt: row.expires_at,
    grantId: row.grant_id,
    revokedAt: row.revoked_at,
    scopes: row.scopes.split(" ").filter(Boolean),
  };
}

function authorizationCode(row: CodeRow): OAuthAuthorizationCode {
  return {
    ...credential(row),
    codeChallenge: row.code_challenge,
    consumedAt: row.consumed_at,
    redirectUri: row.redirect_uri,
  };
}
