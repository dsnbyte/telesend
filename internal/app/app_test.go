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
