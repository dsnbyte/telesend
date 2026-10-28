package store

import (
	"testing"
	"time"
)

func TestUIAuthRepository(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	auth := NewUIAuth(db)

	// Sessions
	token := "session_tok_1"
	if err := auth.CreateSession(token, 1*time.Hour); err != nil {
		t.Fatal(err)
	}

	valid, err := auth.ValidateSession(token)
	if err != nil || !valid {
		t.Fatalf("expected valid session: %v %v", valid, err)
	}

	// Non-existent session
	valid, err = auth.ValidateSession("bad_tok")
	if err != nil || valid {
		t.Fatalf("expected invalid for bad_tok: %v %v", valid, err)
	}

	// Expired session
	expiredToken := "expired_tok"
	if err := auth.CreateSession(expiredToken, -1*time.Minute); err != nil {
		t.Fatal(err)
	}
	valid, err = auth.ValidateSession(expiredToken)
	if err != nil || valid {
		t.Fatalf("expected expired token to be invalid: %v %v", valid, err)
	}

	// Delete session
	if err := auth.DeleteSession(token); err != nil {
		t.Fatal(err)
	}
	valid, _ = auth.ValidateSession(token)
	if valid {
		t.Fatal("expected deleted session to be invalid")
	}

	// Delete all sessions
	_ = auth.CreateSession("t1", time.Hour)
	_ = auth.CreateSession("t2", time.Hour)
	if err := auth.DeleteAllSessions(); err != nil {
		t.Fatal(err)
	}
	v1, _ := auth.ValidateSession("t1")
	v2, _ := auth.ValidateSession("t2")
	if v1 || v2 {
		t.Fatal("expected all sessions deleted")
	}
}

func TestBotsUpdate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bots := NewBots(db)

	_, err = bots.Add(VerifiedBot{TelegramID: "1", Name: "Original", Username: "mybot", Token: "tok1"})
	if err != nil {
		t.Fatal(err)
	}

	// Update name only
	up1, err := bots.Update("mybot", BotInput{Name: "Renamed Bot"})
	if err != nil {
		t.Fatal(err)
	}
	if up1.Name != "Renamed Bot" || up1.Username != "mybot" {
		t.Fatalf("unexpected updated bot: %+v", up1)
	}

	stored, _ := bots.Find("mybot")
	if stored.Token != "tok1" || stored.Name != "Renamed Bot" {
		t.Fatalf("stored bot mismatch: %+v", stored)
	}

	// Update token
	up2, err := bots.Update("mybot", BotInput{Token: "tok2"})
	if err != nil {
		t.Fatal(err)
	}
	if up2.Name != "Renamed Bot" {
		t.Fatalf("name should be unchanged: %+v", up2)
	}
	stored2, _ := bots.Find("mybot")
	if stored2.Token != "tok2" {
		t.Fatalf("stored token mismatch: %+v", stored2)
	}
}
