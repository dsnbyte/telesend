package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHistory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hist := NewHistory(db, 7)
	if hist.RetentionDays() != 7 {
		t.Fatalf("expected retention 7, got %d", hist.RetentionDays())
	}

	thread := 42
	msgID := int64(12345)
	payload := json.RawMessage(`{"text":"hello"}`)

	// Record
	rec, err := hist.Record(HistoryEntry{
		Bot:             "my_bot",
		ChatID:          "-100123",
		Recipient:       "ops",
		MessageThreadID: &thread,
		MessageType:     "text",
		Summary:         "hello",
		Payload:         payload,
		MessageID:       &msgID,
	})
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}
	if rec.ID <= 0 {
		t.Fatalf("expected positive ID, got %d", rec.ID)
	}
	if rec.Status != "delivered" {
		t.Fatalf("expected status delivered, got %s", rec.Status)
	}
	if rec.CreatedAt == "" {
		t.Fatal("expected non-empty CreatedAt")
	}

	// Count
	cnt, err := hist.Count()
	if err != nil || cnt != 1 {
		t.Fatalf("expected count 1, got %d, err %v", cnt, err)
	}

	// Get
	got, err := hist.Get(rec.ID)
	if err != nil || got == nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Summary != "hello" || got.Bot != "my_bot" || got.Recipient != "ops" {
		t.Fatalf("unexpected got: %+v", got)
	}
	if got.MessageThreadID == nil || *got.MessageThreadID != 42 {
		t.Fatalf("unexpected thread ID: %v", got.MessageThreadID)
	}
	if got.MessageID == nil || *got.MessageID != 12345 {
		t.Fatalf("unexpected message ID: %v", got.MessageID)
	}

	// Record a second entry
	rec2, err := hist.Record(HistoryEntry{
		Bot:         "other_bot",
		ChatID:      "987654",
		Recipient:   "alice",
		MessageType: "photo",
		Summary:     "chart photo",
		Payload:     json.RawMessage(`{"caption":"chart photo"}`),
	})
	if err != nil {
		t.Fatalf("record 2 failed: %v", err)
	}

	// List all
	list, err := hist.List(HistoryFilter{})
	if err != nil || len(list) != 2 {
		t.Fatalf("expected 2 items, got %d, err %v", len(list), err)
	}
	// Newest first
	if list[0].ID != rec2.ID || list[1].ID != rec.ID {
		t.Fatalf("expected newest first, got %d then %d", list[0].ID, list[1].ID)
	}

	// Filter by bot
	listBot, err := hist.List(HistoryFilter{Bot: "my_bot"})
	if err != nil || len(listBot) != 1 || listBot[0].ID != rec.ID {
		t.Fatalf("expected 1 item for my_bot, got %d", len(listBot))
	}

	// Filter by type
	listType, err := hist.List(HistoryFilter{MessageType: "photo"})
	if err != nil || len(listType) != 1 || listType[0].ID != rec2.ID {
		t.Fatalf("expected 1 item for photo, got %d", len(listType))
	}

	// Filter by recipient
	listRcpt, err := hist.List(HistoryFilter{Recipient: "alice"})
	if err != nil || len(listRcpt) != 1 || listRcpt[0].ID != rec2.ID {
		t.Fatalf("expected 1 item for alice, got %d", len(listRcpt))
	}

	// Delete one
	ok, err := hist.Delete(rec.ID)
	if err != nil || !ok {
		t.Fatalf("delete failed: %v", err)
	}
	cnt, _ = hist.Count()
	if cnt != 1 {
		t.Fatalf("expected count 1 after delete, got %d", cnt)
	}

	// Clear
	cleared, err := hist.Clear()
	if err != nil || cleared != 1 {
		t.Fatalf("expected cleared 1, got %d, err %v", cleared, err)
	}
	cnt, _ = hist.Count()
	if cnt != 0 {
		t.Fatalf("expected count 0 after clear, got %d", cnt)
	}
}

func TestHistoryAutomaticPruning(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 7 days retention
	hist := NewHistory(db, 7)

	now := time.Now().UTC()
	oldTime := now.AddDate(0, 0, -10).Format("2006-01-02T15:04:05.000Z")
	recentTime := now.AddDate(0, 0, -3).Format("2006-01-02T15:04:05.000Z")
	todayTime := now.Format("2006-01-02T15:04:05.000Z")

	// Directly insert records with specific timestamps to test pruning
	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "old_rcpt", "text", "old message", "{}", "delivered", oldTime,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "recent_rcpt", "text", "recent message", "{}", "delivered", recentTime,
	)
	if err != nil {
		t.Fatal(err)
	}

	cnt, _ := hist.Count()
	if cnt != 2 {
		t.Fatalf("expected 2 records before prune, got %d", cnt)
	}

	// Explicit prune
	pruned, err := hist.Prune(now)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 record pruned, got %d", pruned)
	}

	cnt, _ = hist.Count()
	if cnt != 1 {
		t.Fatalf("expected 1 record remaining, got %d", cnt)
	}

	// Insert another old record to test opportunistic pruning on Record
	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "another_old", "text", "another old", "{}", "delivered", oldTime,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Record a new message - should trigger automatic pruning
	_, err = hist.Record(HistoryEntry{
		Bot:         "bot1",
		ChatID:      "123",
		Recipient:   "today_rcpt",
		MessageType: "text",
		Summary:     "today message",
		CreatedAt:   todayTime,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Old record should be gone, only recent and today should remain (2 total)
	cnt, _ = hist.Count()
	if cnt != 2 {
		t.Fatalf("expected 2 records after opportunistic record prune, got %d", cnt)
	}

	// Insert another old record to test opportunistic pruning on List
	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "yet_another_old", "text", "yet another old", "{}", "delivered", oldTime,
	)
	if err != nil {
		t.Fatal(err)
	}

	list, err := hist.List(HistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 records after opportunistic list prune, got %d", len(list))
	}
}

func TestHistoryCustomRetention(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 2 days retention
	hist := NewHistory(db, 2)
	if hist.RetentionDays() != 2 {
		t.Fatalf("expected retention 2, got %d", hist.RetentionDays())
	}

	now := time.Now().UTC()
	fourDaysAgo := now.AddDate(0, 0, -4).Format("2006-01-02T15:04:05.000Z")
	oneDayAgo := now.AddDate(0, 0, -1).Format("2006-01-02T15:04:05.000Z")

	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "r1", "text", "msg1", "{}", "delivered", fourDaysAgo,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_type, summary, payload, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"bot1", "123", "r2", "text", "msg2", "{}", "delivered", oneDayAgo,
	)
	if err != nil {
		t.Fatal(err)
	}

	pruned, err := hist.Prune(now)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned with 2-day retention, got %d", pruned)
	}

	cnt, _ := hist.Count()
	if cnt != 1 {
		t.Fatalf("expected 1 remaining, got %d", cnt)
	}
}
