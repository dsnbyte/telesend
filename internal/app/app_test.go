package app_test

import (
	"context"
	"testing"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/telegram"
	"github.com/dsnbyte/telesend/internal/testutil"
)

func TestAppEndToEnd(t *testing.T) {
	a, tg := testutil.NewApp(t)

	// 1. Register bot
	b, err := a.RegisterBot(context.Background(), "bot_token")
	if err != nil {
		t.Fatal(err)
	}
	if b.Username != "default_bot" || !b.IsDefault {
		t.Fatalf("bot: %+v", b)
	}

	// 2. Manage alias
	thread := 12
	alias, err := a.Aliases.Create(store.AliasInput{Name: "ops", ChatID: "-100123", MessageThreadID: &thread})
	if err != nil {
		t.Fatal(err)
	}
	if alias.Type != store.TypeGroup {
		t.Fatalf("type: %s", alias.Type)
	}

	// 3. Resolve recipient - alias vs direct chat ID
	rcpt1, err := a.ResolveRecipient("ops", 0)
	if err != nil || rcpt1.ChatID != "-100123" || rcpt1.ThreadID != 12 {
		t.Fatalf("rcpt1: %+v %v", rcpt1, err)
	}
	// Direct chat ID
	rcpt2, err := a.ResolveRecipient("-100999", 5)
	if err != nil || rcpt2.ChatID != "-100999" || rcpt2.ThreadID != 5 {
		t.Fatalf("rcpt2: %+v %v", rcpt2, err)
	}

	// 4. Send message to alias
	d1, err := a.Send(context.Background(), app.SendRequest{
		Type:    "text",
		To:      "ops",
		Payload: map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatal("send to alias:", err)
	}
	if d1.ChatID != "-100123" || d1.ThreadID != 12 || d1.MessageID == 0 {
		t.Fatalf("d1: %+v", d1)
	}

	// 5. Send message directly to chat ID
	d2, err := a.Send(context.Background(), app.SendRequest{
		Type:     "text",
		To:       "-100555",
		ThreadID: 3,
		Payload:  map[string]any{"text": "direct chat id test"},
	})
	if err != nil {
		t.Fatal("send to chat id:", err)
	}
	if d2.ChatID != "-100555" || d2.ThreadID != 3 {
		t.Fatalf("d2: %+v", d2)
	}

	// 6. Deliver all catalog operations
	for _, op := range telegram.Catalog {
		res, err := a.Send(context.Background(), app.SendRequest{
			Type:    op.Type,
			To:      "ops",
			Payload: op.Example,
		})
		if err != nil {
			t.Fatalf("send %s: %v", op.Type, err)
		}
		if res.MessageID == 0 {
			t.Fatalf("expected message id for %s", op.Type)
		}
	}
	if len(tg.Calls()) < len(telegram.Catalog)+2 {
		t.Fatalf("calls: %d", len(tg.Calls()))
	}
}

func TestResolveRecipient(t *testing.T) {
	a, _ := testutil.NewApp(t)

	thread := 12
	_, err := a.Aliases.Create(store.AliasInput{Name: "ops", ChatID: "-100123", MessageThreadID: &thread})
	if err != nil {
		t.Fatal(err)
	}

	teamThread := 20
	_, err = a.Aliases.Create(store.AliasInput{Name: "ops team", ChatID: "-100456", MessageThreadID: &teamThread})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Alias configured thread is used when no argument or inline thread is provided
	r, err := a.ResolveRecipient("ops", 0)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 12 {
		t.Fatalf("alias default: %+v %v", r, err)
	}

	// 2. Passing a thread ID argument overrides the thread ID configured for an alias
	r, err = a.ResolveRecipient("ops", 99)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 99 {
		t.Fatalf("alias arg override: %+v %v", r, err)
	}

	// 3. Specifying a thread ID by appending #<thread_id> after the alias
	r, err = a.ResolveRecipient("ops#42", 0)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 42 {
		t.Fatalf("alias inline thread: %+v %v", r, err)
	}

	// 4. Specifying a thread ID by appending #<thread_id> after the chat ID
	r, err = a.ResolveRecipient("-100123#42", 0)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 42 {
		t.Fatalf("chat id inline thread: %+v %v", r, err)
	}

	// 5. Specifying a thread ID with an alias containing spaces
	r, err = a.ResolveRecipient("ops team#88", 0)
	if err != nil || r.ChatID != "-100456" || r.ThreadID != 88 {
		t.Fatalf("alias with spaces inline thread: %+v %v", r, err)
	}

	// 6. Explicit thread argument overrides #<thread_id>
	r, err = a.ResolveRecipient("ops#42", 99)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 99 {
		t.Fatalf("arg override inline thread alias: %+v %v", r, err)
	}
	r, err = a.ResolveRecipient("-100123#42", 99)
	if err != nil || r.ChatID != "-100123" || r.ThreadID != 99 {
		t.Fatalf("arg override inline thread chat id: %+v %v", r, err)
	}

	// 7. Invalid thread IDs return validation error
	invalidTargets := []string{
		"ops#abc", "ops#0", "ops#-5", "ops#", "ops#1#2",
		"-100123#abc", "-100123#0", "-100123#-1", "-100123#",
	}
	for _, target := range invalidTargets {
		_, err := a.ResolveRecipient(target, 0)
		if err == nil {
			t.Fatalf("expected error for invalid target %q", target)
		}
	}

	// 8. Negative explicitThread returns validation error
	if _, err := a.ResolveRecipient("ops", -1); err == nil {
		t.Fatal("expected error for negative explicitThread")
	}

	// 9. Unknown alias returns not found error
	if _, err := a.ResolveRecipient("nonexistent#42", 0); err == nil || err.Error() != `Recipient alias "nonexistent" was not found` {
		t.Fatalf("expected not found error: %v", err)
	}
}

func TestResolveRecipientPrivateChat(t *testing.T) {
	a, _ := testutil.NewApp(t)

	_, err := a.Aliases.Create(store.AliasInput{Name: "alice", ChatID: "123456"})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Direct private chat ID ignores #<thread_id>
	r, err := a.ResolveRecipient("123456#42", 0)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("direct private chat inline thread: %+v %v", r, err)
	}

	// 2. Direct private chat ID ignores explicit thread argument
	r, err = a.ResolveRecipient("123456", 42)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("direct private chat arg thread: %+v %v", r, err)
	}

	// 3. Direct private chat ID ignores both
	r, err = a.ResolveRecipient("123456#42", 99)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("direct private chat both threads: %+v %v", r, err)
	}

	// 4. Alias pointing to private chat ignores #<thread_id>
	r, err = a.ResolveRecipient("alice#42", 0)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("private alias inline thread: %+v %v", r, err)
	}

	// 5. Alias pointing to private chat ignores explicit thread argument
	r, err = a.ResolveRecipient("alice", 42)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("private alias arg thread: %+v %v", r, err)
	}

	// 6. Alias pointing to private chat ignores both
	r, err = a.ResolveRecipient("alice#42", 99)
	if err != nil || r.ChatID != "123456" || r.ThreadID != 0 {
		t.Fatalf("private alias both threads: %+v %v", r, err)
	}

	// 7. Invalid thread ID format on private chat still returns validation error
	if _, err := a.ResolveRecipient("123456#abc", 0); err == nil {
		t.Fatal("expected validation error for 123456#abc")
	}
	if _, err := a.ResolveRecipient("alice#abc", 0); err == nil {
		t.Fatal("expected validation error for alice#abc")
	}
}

func TestSendWithThreadSyntax(t *testing.T) {
	a, tg := testutil.NewApp(t)

	_, err := a.RegisterBot(context.Background(), "bot_token")
	if err != nil {
		t.Fatal(err)
	}

	thread := 10
	_, err = a.Aliases.Create(store.AliasInput{Name: "ops", ChatID: "-100123", MessageThreadID: &thread})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Aliases.Create(store.AliasInput{Name: "alice", ChatID: "123456"})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Send to ops#42 sets message_thread_id to 42
	d, err := a.Send(context.Background(), app.SendRequest{
		Type:    "text",
		To:      "ops#42",
		Payload: map[string]any{"text": "msg to ops thread 42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ChatID != "-100123" || d.ThreadID != 42 {
		t.Fatalf("expected thread 42: %+v", d)
	}
	lastCall := tg.Calls()[len(tg.Calls())-1]
	if lastCall.JSON["message_thread_id"] == nil {
		t.Fatalf("expected message_thread_id in call: %+v", lastCall.JSON)
	}

	// 2. Send to ops#42 with ThreadID 99 argument overrides to 99
	d, err = a.Send(context.Background(), app.SendRequest{
		Type:     "text",
		To:       "ops#42",
		ThreadID: 99,
		Payload:  map[string]any{"text": "msg to ops thread 99 override"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ChatID != "-100123" || d.ThreadID != 99 {
		t.Fatalf("expected thread 99: %+v", d)
	}

	// 3. Send to private chat alias alice#42 ignores thread ID
	d, err = a.Send(context.Background(), app.SendRequest{
		Type:    "text",
		To:      "alice#42",
		Payload: map[string]any{"text": "msg to alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ChatID != "123456" || d.ThreadID != 0 {
		t.Fatalf("expected thread 0 for private chat alias: %+v", d)
	}
	lastCall = tg.Calls()[len(tg.Calls())-1]
	if lastCall.JSON["message_thread_id"] != nil {
		t.Fatalf("expected no message_thread_id for private chat: %+v", lastCall.JSON)
	}

	// 4. Send to direct private chat ID 123456#42 ignores thread ID
	d, err = a.Send(context.Background(), app.SendRequest{
		Type:    "text",
		To:      "123456#42",
		Payload: map[string]any{"text": "msg to direct private chat"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ChatID != "123456" || d.ThreadID != 0 {
		t.Fatalf("expected thread 0 for direct private chat: %+v", d)
	}
	lastCall = tg.Calls()[len(tg.Calls())-1]
	if lastCall.JSON["message_thread_id"] != nil {
		t.Fatalf("expected no message_thread_id for private chat: %+v", lastCall.JSON)
	}
}

func TestAppUpdateBot(t *testing.T) {
	a, _ := testutil.NewApp(t)
	ctx := context.Background()

	// 1. Register bot
	b, err := a.RegisterBot(ctx, "bot_token")
	if err != nil {
		t.Fatal(err)
	}
	if b.Username != "default_bot" {
		t.Fatalf("initial username: %s", b.Username)
	}

	// 2. Update name only
	updated, err := a.UpdateBot(ctx, "default_bot", store.BotInput{Name: "Custom Bot Name"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Custom Bot Name" {
		t.Fatalf("updated name: %s", updated.Name)
	}

	// 3. Update with valid token
	updatedToken, err := a.UpdateBot(ctx, "default_bot", store.BotInput{Token: "bot_token", Name: "Fresh Name"})
	if err != nil {
		t.Fatal(err)
	}
	if updatedToken.Name != "Fresh Name" {
		t.Fatalf("updatedToken name: %s", updatedToken.Name)
	}

	// 4. Update with token for different bot fails
	_, err = a.UpdateBot(ctx, "default_bot", store.BotInput{Token: "two_token"})
	if err == nil {
		t.Fatal("expected error updating token to a different bot ID")
	}

	// 5. Update non-existent bot fails
	_, err = a.UpdateBot(ctx, "ghost_bot", store.BotInput{Name: "Ghost"})
	if err == nil {
		t.Fatal("expected error updating non-existent bot")
	}
}

func TestAppDeliveryHistory(t *testing.T) {
	a, _ := testutil.NewApp(t)
	ctx := context.Background()

	_, err := a.RegisterBot(ctx, "bot_token")
	if err != nil {
		t.Fatal(err)
	}

	alias, err := a.Aliases.Create(store.AliasInput{Name: "alerts", ChatID: "-100555"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Send(ctx, app.SendRequest{
		Type:    "text",
		To:      alias.Name,
		Payload: map[string]any{"text": "disk full warning"},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := a.History.List(store.HistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Bot != "default_bot" {
		t.Fatalf("expected bot default_bot, got %s", e.Bot)
	}
	if e.Recipient != "alerts" {
		t.Fatalf("expected recipient alerts, got %s", e.Recipient)
	}
	if e.ChatID != "-100555" {
		t.Fatalf("expected chat_id -100555, got %s", e.ChatID)
	}
	if e.MessageType != "text" {
		t.Fatalf("expected type text, got %s", e.MessageType)
	}
	if e.Summary != "disk full warning" {
		t.Fatalf("expected summary disk full warning, got %s", e.Summary)
	}
	if e.Status != "delivered" {
		t.Fatalf("expected status delivered, got %s", e.Status)
	}
}


