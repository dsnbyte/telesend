package secret

import (
	"strings"
	"testing"
)

func TestHashVerifyAndEncode(t *testing.T) {
	hash, err := Hash("correct horse")
	if err != nil || !strings.HasPrefix(hash, "$pbkdf2-sha256$") {
		t.Fatal(hash, err)
	}
	if !Verify("correct horse", hash) || Verify("wrong", hash) {
		t.Fatal("verify mismatch")
	}
	enc := Encode(hash)
	if strings.ContainsAny(enc, "$=+/") {
		t.Fatal("not dotenv safe:", enc)
	}
	for _, in := range []string{hash, enc} {
		got, err := Parse(in)
		if err != nil || got != hash {
			t.Fatal(got, err)
		}
	}
	for _, bad := range []string{"", "$argon2id$x", "!!!", Encode("nope")} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestVerifier(t *testing.T) {
	if v, err := NewVerifier("", "  "); v != nil || err != nil {
		t.Fatal("expected unconfigured")
	}
	plain, _ := NewVerifier("", "key")
	if !plain.Verify("key") || plain.Verify("nope") {
		t.Fatal("plain verifier")
	}
	hash, _ := Hash("key")
	hv, err := NewVerifier(Encode(hash), "ignored")
	if err != nil || !hv.Verify("key") || !hv.Verify("key") || hv.Verify("nope") {
		t.Fatal("hash verifier", err)
	}
	if _, err := NewVerifier("garbage", "key"); err == nil {
		t.Fatal("invalid hash must not fall back to plaintext")
	}
}
