package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# c\nTS_A=1\nexport TS_B=\"two words\"\nTS_C=keep\n"), 0o600)
	t.Setenv("TS_C", "existing")
	os.Unsetenv("TS_A")
	os.Unsetenv("TS_B")
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TS_A") != "1" || os.Getenv("TS_B") != "two words" || os.Getenv("TS_C") != "existing" {
		t.Fatal("unexpected environment")
	}
	if err := Load(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
}
