package store

import (
	"path/filepath"
	"testing"
)

func newTestDB(t testing.TB) *Aliases {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewAliases(db)
}

func TestBotsRepository(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bots := NewBots(db)

	b1, err := bots.Add(VerifiedBot{TelegramID: "1", Name: "First", Username: "first_bot", Token: "tok1"})
	if err != nil {
		t.Fatal(err)
	}
	if !b1.IsDefault || b1.Username != "first_bot" {
		t.Fatalf("expected first default: %+v", b1)
	}

	b2, err := bots.Add(VerifiedBot{TelegramID: "2", Name: "Second", Username: "@second_bot", Token: "tok2"})
	if err != nil {
		t.Fatal(err)
	}
	if b2.IsDefault {
		t.Fatal("second bot must not be default")
	}

	// Reject duplicates
	if _, err := bots.Add(VerifiedBot{TelegramID: "1", Name: "Dup", Username: "other", Token: "t"}); err == nil {
		t.Fatal("expected dup telegram ID error")
	}
	if _, err := bots.Add(VerifiedBot{TelegramID: "3", Name: "Dup", Username: "first_bot", Token: "t"}); err == nil {
		t.Fatal("expected dup username error")
	}

	// List
	list, err := bots.List()
	if err != nil || len(list) != 2 || list[0].Username != "first_bot" {
		t.Fatalf("list: %+v %v", list, err)
	}

	// SetDefault
	switched, err := bots.SetDefault("second_bot")
	if err != nil || !switched.IsDefault {
		t.Fatalf("set default: %+v %v", switched, err)
	}
	def, _ := bots.Default()
	if def.Username != "second_bot" {
		t.Fatal("expected second default")
	}

	// Remove default when another exists should fail
	if _, err := bots.Remove("second_bot"); err == nil {
		t.Fatal("expected error removing default while alternatives exist")
	}

	// Remove non-default
	if _, err := bots.Remove("first_bot"); err != nil {
		t.Fatal("remove non-default:", err)
	}
	// Now removing the only remaining bot succeeds
	if _, err := bots.Remove("second_bot"); err != nil {
		t.Fatal("remove last bot:", err)
	}
	remList, _ := bots.List()
	if len(remList) != 0 {
		t.Fatal("expected empty bots")
	}
}

func TestAliasesRepository(t *testing.T) {
	aliases := newTestDB(t)

	thread := 42
	a1, err := aliases.Create(AliasInput{Name: "ops team", ChatID: "-100123", MessageThreadID: &thread})
	if err != nil {
		t.Fatal(err)
	}
	if a1.Name != "ops team" || a1.Type != TypeGroup || a1.MessageThreadID == nil || *a1.MessageThreadID != 42 {
		t.Fatalf("created: %+v", a1)
	}

	a2, err := aliases.Create(AliasInput{Name: "alice", ChatID: "12345"})
	if err != nil {
		t.Fatal(err)
	}
	if a2.Type != TypePrivate {
		t.Fatalf("expected private type: %+v", a2)
	}

	// Duplicates
	if _, err := aliases.Create(AliasInput{Name: "OPS TEAM", ChatID: "-999"}); err == nil {
		t.Fatal("expected duplicate name error")
	}

	// Validation
	if _, err := aliases.Create(AliasInput{Name: "bad/name", ChatID: "123"}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if _, err := aliases.Create(AliasInput{Name: "good", ChatID: "abc"}); err == nil {
		t.Fatal("expected invalid chat ID error")
	}

	// Find
	found, err := aliases.Find("ops team")
	if err != nil || found == nil || found.ChatID != "-100123" {
		t.Fatalf("find: %+v %v", found, err)
	}

	// Update
	upThread := 99
	updated, err := aliases.Update("ops team", AliasInput{Name: "ops-updated", ChatID: "-100999", MessageThreadID: &upThread})
	if err != nil || updated.Name != "ops-updated" || *updated.MessageThreadID != 99 {
		t.Fatalf("update: %+v %v", updated, err)
	}

	// Remove
	removed, err := aliases.Remove("ops-updated")
	if err != nil || removed.Name != "ops-updated" {
		t.Fatalf("remove: %+v %v", removed, err)
	}
	list, _ := aliases.List()
	if len(list) != 1 || list[0].Name != "alice" {
		t.Fatalf("remaining: %+v", list)
	}

	// Filtered list
	_, err = aliases.Create(AliasInput{Name: "alerts", ChatID: "-100789"})
	if err != nil {
		t.Fatal(err)
	}
	groupList, err := aliases.List(TypeGroup)
	if err != nil || len(groupList) != 1 || groupList[0].Name != "alerts" {
		t.Fatalf("group list: %+v %v", groupList, err)
	}
	privList, err := aliases.List(TypePrivate)
	if err != nil || len(privList) != 1 || privList[0].Name != "alice" {
		t.Fatalf("private list: %+v %v", privList, err)
	}
	if _, err := aliases.List("invalid"); err == nil {
		t.Fatal("expected error for invalid filter")
	}
}

func TestOAuthRepository(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oauth := NewOAuth(db)

	client := OAuthClient{ID: "client_1", Name: "Test Client", RedirectURIs: []string{"https://example.com/cb"}}
	if err := oauth.RegisterClient(client, 100); err != nil {
		t.Fatal(err)
	}
	found, err := oauth.FindClient("client_1")
	if err != nil || found == nil || found.Name != "Test Client" || len(found.RedirectURIs) != 1 {
		t.Fatalf("find client: %+v %v", found, err)
	}

	if err := oauth.CreateGrant("grant_1", "client_1", []string{"mcp:read", "mcp:send"}, 100); err != nil {
		t.Fatal(err)
	}

	// Authorization code single-use
	codeDigest := "digest_code_1"
	if err := oauth.StoreAuthorizationCode(codeDigest, "grant_1", "https://example.com/cb", "challenge_1", 200); err != nil {
		t.Fatal(err)
	}
	consumed, err := oauth.ConsumeAuthorizationCode(codeDigest, 150)
	if err != nil || consumed == nil || consumed.GrantID != "grant_1" || consumed.CodeChallenge != "challenge_1" {
		t.Fatalf("consume code: %+v %v", consumed, err)
	}
	// Second consume must return nil
	second, err := oauth.ConsumeAuthorizationCode(codeDigest, 150)
	if err != nil || second != nil {
		t.Fatalf("expected nil on second consume: %+v %v", second, err)
	}

	// Store tokens and find
	tokens := TokenPair{
		AccessDigest:     "access_d_1",
		AccessExpiresAt:  300,
		GrantID:          "grant_1",
		RefreshDigest:    "refresh_d_1",
		RefreshExpiresAt: 1000,
	}
	if err := oauth.StoreTokens(tokens); err != nil {
		t.Fatal(err)
	}
	cred, err := oauth.FindAccessToken("access_d_1")
	if err != nil || cred == nil || cred.GrantID != "grant_1" || cred.Revoked {
		t.Fatalf("find access token: %+v %v", cred, err)
	}

	// Rotate refresh token
	rotated, err := oauth.RotateRefreshToken("refresh_d_1", "client_1", TokenPair{
		AccessDigest:     "access_d_2",
		AccessExpiresAt:  400,
		RefreshDigest:    "refresh_d_2",
		RefreshExpiresAt: 1100,
	}, 250)
	if err != nil || rotated == nil || rotated.GrantID != "grant_1" {
		t.Fatalf("rotate refresh token: %+v %v", rotated, err)
	}
	// Old refresh token cannot be rotated again
	oldRotate, err := oauth.RotateRefreshToken("refresh_d_1", "client_1", TokenPair{}, 260)
	if err != nil || oldRotate != nil {
		t.Fatalf("expected old refresh token rotation to fail: %+v %v", oldRotate, err)
	}

	// Revoke grant
	revoked, err := oauth.RevokeGrantByToken("access_d_2", 300)
	if err != nil || !revoked {
		t.Fatalf("revoke: %v %v", revoked, err)
	}
	cred2, _ := oauth.FindAccessToken("access_d_2")
	if cred2 == nil || !cred2.Revoked {
		t.Fatalf("expected revoked token: %+v", cred2)
	}
}

func TestFileDatabaseCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
