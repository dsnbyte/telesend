package store_test

import (
	"strings"
	"testing"

	"github.com/dsnbyte/telesend/internal/store"
)

func TestExportAndImportSQL(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bots := store.NewBots(db)
	aliases := store.NewAliases(db)

	// Add test bot and alias
	bot, err := bots.Add(store.VerifiedBot{
		TelegramID: "111222333",
		Name:       "Test Bot",
		Username:   "test_bot",
		Token:      "secret_token_123",
	})
	if err != nil {
		t.Fatal(err)
	}

	thread := 42
	alias, err := aliases.Create(store.AliasInput{
		Name:            "team",
		ChatID:          "-100123456789",
		MessageThreadID: &thread,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Export SQL
	sqlDump, err := store.ExportSQL(db)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	if !strings.Contains(sqlDump, "INSERT INTO \"bots\"") {
		t.Fatal("expected SQL dump to contain INSERT INTO \"bots\"")
	}
	if !strings.Contains(sqlDump, "test_bot") {
		t.Fatal("expected SQL dump to contain test_bot")
	}
	if !strings.Contains(sqlDump, "INSERT INTO \"aliases\"") {
		t.Fatal("expected SQL dump to contain INSERT INTO \"aliases\"")
	}
	if !strings.Contains(sqlDump, "team") {
		t.Fatal("expected SQL dump to contain team")
	}

	// Create a new database and import the dump
	db2, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	if err := store.ImportSQL(db2, sqlDump); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Verify imported data in db2
	bots2 := store.NewBots(db2)
	bList, err := bots2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bList) != 1 || bList[0].Username != bot.Username {
		t.Fatalf("expected imported bot %s, got %+v", bot.Username, bList)
	}

	aliases2 := store.NewAliases(db2)
	aList, err := aliases2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(aList) != 1 || aList[0].Name != alias.Name {
		t.Fatalf("expected imported alias %s, got %+v", alias.Name, aList)
	}

	// Verify overwriting existing data
	// Add new bot to db2
	_, err = bots2.Add(store.VerifiedBot{
		TelegramID: "999888777",
		Name:       "Another Bot",
		Username:   "another_bot",
		Token:      "another_secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Re-import the original dump: another_bot should be overwritten/gone!
	if err := store.ImportSQL(db2, sqlDump); err != nil {
		t.Fatalf("re-import failed: %v", err)
	}

	bListAfter, err := bots2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bListAfter) != 1 || bListAfter[0].Username != "test_bot" {
		t.Fatalf("expected overwrite to leave only test_bot, got %+v", bListAfter)
	}
}
