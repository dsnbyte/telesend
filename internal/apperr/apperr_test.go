package apperr

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactConfiguredSecrets(t *testing.T) {
	if got := Redact("key=abc123 done", "abc123"); got != "key=[REDACTED] done" {
		t.Fatal(got)
	}
}

func TestRedactTelegramTokens(t *testing.T) {
	token := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef"
	for _, in := range []string{"bad " + token, "https://api.telegram.org/bot" + token + "/getMe"} {
		if got := Redact(in); strings.Contains(got, token) || strings.Contains(got, "ABCDEFGH") {
			t.Fatalf("leaked: %s", got)
		}
	}
}

func TestFrom(t *testing.T) {
	e := From(New(NotFound, "x"))
	if e.Code != NotFound {
		t.Fatal(e.Code)
	}
	if From(errors.New("boom")).Code != Internal {
		t.Fatal("expected internal")
	}
}
