package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// OAuthClient is a dynamically registered public client.
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
}

// Credential describes an access token, refresh token, or authorization code.
type Credential struct {
	ClientID  string
	ExpiresAt int64
	GrantID   string
	Revoked   bool
	Scopes    []string
}

// AuthorizationCode is a consumed authorization code.
type AuthorizationCode struct {
	Credential
	CodeChallenge string
	RedirectURI   string
}

// TokenPair holds digests of a newly issued access and refresh token.
type TokenPair struct {
	AccessDigest     string
	AccessExpiresAt  int64
	GrantID          string
	RefreshDigest    string
	RefreshExpiresAt int64
}

// OAuth persists OAuth clients, grants, codes, and token digests.
type OAuth struct{ db *sql.DB }

// NewOAuth wraps db.
func NewOAuth(db *sql.DB) *OAuth { return &OAuth{db} }

// RegisterClient stores a client.
func (r *OAuth) RegisterClient(c OAuthClient, now int64) error {
	uris, _ := json.Marshal(c.RedirectURIs)
	_, err := r.db.Exec("INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, ?, ?, ?)",
		c.ID, c.Name, string(uris), now)
	return err
}

// FindClient returns the client or nil.
func (r *OAuth) FindClient(id string) (*OAuthClient, error) {
	var c OAuthClient
	var uris string
	err := r.db.QueryRow("SELECT id, name, redirect_uris FROM oauth_clients WHERE id = ?", id).Scan(&c.ID, &c.Name, &uris)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(uris), &c.RedirectURIs); err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateGrant stores a grant for a client.
func (r *OAuth) CreateGrant(id, clientID string, scopes []string, now int64) error {
	_, err := r.db.Exec("INSERT INTO oauth_grants (id, client_id, scopes, created_at) VALUES (?, ?, ?, ?)",
		id, clientID, strings.Join(scopes, " "), now)
	return err
}

// StoreAuthorizationCode stores a single-use code digest.
func (r *OAuth) StoreAuthorizationCode(digest, grantID, redirectURI, challenge string, expiresAt int64) error {
	_, err := r.db.Exec(
		`INSERT INTO oauth_authorization_codes (digest, grant_id, redirect_uri, code_challenge, expires_at)
		 VALUES (?, ?, ?, ?, ?)`, digest, grantID, redirectURI, challenge, expiresAt)
	return err
}

type credentialRow struct {
	clientID  string
	expiresAt int64
	grantID   string
	revokedAt sql.NullInt64
	scopes    string
}

func (c credentialRow) credential() Credential {
	return Credential{
		ClientID: c.clientID, ExpiresAt: c.expiresAt, GrantID: c.grantID,
		Revoked: c.revokedAt.Valid, Scopes: strings.Fields(c.scopes),
	}
}

// ConsumeAuthorizationCode marks a valid code consumed and returns it.
func (r *OAuth) ConsumeAuthorizationCode(digest string, now int64) (*AuthorizationCode, error) {
	var out *AuthorizationCode
	err := inTx(r.db, func(tx *sql.Tx) error {
		var row credentialRow
		var redirect, challenge string
		var consumed sql.NullInt64
		err := tx.QueryRow(
			`SELECT c.redirect_uri, c.code_challenge, c.expires_at, c.consumed_at,
			        g.id, g.client_id, g.scopes, g.revoked_at
			   FROM oauth_authorization_codes c
			   JOIN oauth_grants g ON g.id = c.grant_id
			  WHERE c.digest = ?`, digest).
			Scan(&redirect, &challenge, &row.expiresAt, &consumed, &row.grantID, &row.clientID, &row.scopes, &row.revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if consumed.Valid || row.expiresAt <= now || row.revokedAt.Valid {
			return nil
		}
		if _, err := tx.Exec("UPDATE oauth_authorization_codes SET consumed_at = ? WHERE digest = ?", now, digest); err != nil {
			return err
		}
		out = &AuthorizationCode{Credential: row.credential(), CodeChallenge: challenge, RedirectURI: redirect}
		return nil
	})
	return out, err
}

func storeTokens(tx *sql.Tx, t TokenPair) error {
	if _, err := tx.Exec("INSERT INTO oauth_access_tokens (digest, grant_id, expires_at) VALUES (?, ?, ?)",
		t.AccessDigest, t.GrantID, t.AccessExpiresAt); err != nil {
		return err
	}
	_, err := tx.Exec("INSERT INTO oauth_refresh_tokens (digest, grant_id, expires_at) VALUES (?, ?, ?)",
		t.RefreshDigest, t.GrantID, t.RefreshExpiresAt)
	return err
}

// StoreTokens persists a token pair.
func (r *OAuth) StoreTokens(t TokenPair) error {
	return inTx(r.db, func(tx *sql.Tx) error { return storeTokens(tx, t) })
}

// FindAccessToken looks up an access token digest.
func (r *OAuth) FindAccessToken(digest string) (*Credential, error) {
	var row credentialRow
	err := r.db.QueryRow(
		`SELECT t.expires_at, COALESCE(t.revoked_at, g.revoked_at), g.id, g.client_id, g.scopes
		   FROM oauth_access_tokens t
		   JOIN oauth_grants g ON g.id = t.grant_id
		  WHERE t.digest = ?`, digest).
		Scan(&row.expiresAt, &row.revokedAt, &row.grantID, &row.clientID, &row.scopes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := row.credential()
	return &c, nil
}

// RotateRefreshToken consumes a refresh token and stores its replacement pair.
// next.GrantID is ignored; the grant of the old token is used.
func (r *OAuth) RotateRefreshToken(digest, clientID string, next TokenPair, now int64) (*Credential, error) {
	var out *Credential
	err := inTx(r.db, func(tx *sql.Tx) error {
		var row credentialRow
		err := tx.QueryRow(
			`SELECT t.expires_at, COALESCE(t.revoked_at, g.revoked_at), g.id, g.client_id, g.scopes
			   FROM oauth_refresh_tokens t
			   JOIN oauth_grants g ON g.id = t.grant_id
			  WHERE t.digest = ? AND g.client_id = ? AND t.replaced_by_digest IS NULL`, digest, clientID).
			Scan(&row.expiresAt, &row.revokedAt, &row.grantID, &row.clientID, &row.scopes)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.revokedAt.Valid || row.expiresAt <= now {
			return nil
		}
		if _, err := tx.Exec("UPDATE oauth_refresh_tokens SET revoked_at = ?, replaced_by_digest = ? WHERE digest = ?",
			now, next.RefreshDigest, digest); err != nil {
			return err
		}
		next.GrantID = row.grantID
		if err := storeTokens(tx, next); err != nil {
			return err
		}
		c := row.credential()
		out = &c
		return nil
	})
	return out, err
}

// RevokeGrantByToken revokes the grant owning an access or refresh token.
func (r *OAuth) RevokeGrantByToken(digest string, now int64) (bool, error) {
	revoked := false
	err := inTx(r.db, func(tx *sql.Tx) error {
		var grantID string
		err := tx.QueryRow(
			`SELECT grant_id FROM oauth_access_tokens WHERE digest = ?
			 UNION SELECT grant_id FROM oauth_refresh_tokens WHERE digest = ?`, digest, digest).Scan(&grantID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE oauth_grants SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?", now, grantID); err != nil {
			return err
		}
		revoked = true
		return nil
	})
	return revoked, err
}
