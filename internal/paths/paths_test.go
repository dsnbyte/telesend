package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolve(t *testing.T) {
	p, err := Resolve(env(map[string]string{"HOME": "/h", "XDG_DATA_HOME": "/x/d", "XDG_CONFIG_HOME": "/x/c"}), "linux")
	if err != nil || p.DatabaseFile != "/x/d/telesend/telesend.db" || p.ConfigFile != "/x/c/telesend/config.json" {
		t.Fatalf("%+v %v", p, err)
	}
	p, _ = Resolve(env(map[string]string{"HOME": "/h"}), "linux")
	if p.DatabaseFile != "/h/.local/share/telesend/telesend.db" {
		t.Fatal(p.DatabaseFile)
	}
	p, _ = Resolve(env(map[string]string{"HOME": "/h"}), "darwin")
	if p.DataDir != "/h/Library/Application Support/telesend" {
		t.Fatal(p.DataDir)
	}
	p, _ = Resolve(env(map[string]string{"LOCALAPPDATA": `C:\L`, "APPDATA": `C:\R`}), "windows")
	if p.DataDir != filepath.Join(`C:\L`, "telesend") {
		t.Fatal(p.DataDir)
	}
	if _, err := Resolve(env(nil), "linux"); err == nil {
		t.Fatal("expected HOME error")
	}
}

func TestForDir(t *testing.T) {
	p := ForDir("/custom/data")
	if p.DataDir != "/custom/data" {
		t.Errorf("expected DataDir /custom/data, got %s", p.DataDir)
	}
	if p.DatabaseFile != "/custom/data/telesend.db" {
		t.Errorf("expected DatabaseFile /custom/data/telesend.db, got %s", p.DatabaseFile)
	}
	if p.ConfigFile != "/custom/data/config.json" {
		t.Errorf("expected ConfigFile /custom/data/config.json, got %s", p.ConfigFile)
	}
}

func TestDefault(t *testing.T) {
	// Normal default
	p, err := Default()
	if err != nil {
		t.Fatalf("Default failed: %v", err)
	}
	if !filepath.IsAbs(p.DatabaseFile) {
		t.Errorf("DatabaseFile should be absolute, got %s", p.DatabaseFile)
	}
	if filepath.Base(p.DatabaseFile) != "telesend.db" {
		t.Errorf("expected telesend.db filename, got %s", filepath.Base(p.DatabaseFile))
	}
	if filepath.Base(filepath.Dir(p.DatabaseFile)) != "data" {
		t.Errorf("expected parent dir 'data', got %s", filepath.Base(filepath.Dir(p.DatabaseFile)))
	}

	// Override with TELESEND_DATA_DIR
	t.Setenv("TELESEND_DATA_DIR", "/override/path")
	p2, err := Default()
	if err != nil {
		t.Fatalf("Default with env failed: %v", err)
	}
	if p2.DataDir != "/override/path" || p2.DatabaseFile != "/override/path/telesend.db" {
		t.Errorf("unexpected paths with env override: %+v", p2)
	}
}

func TestEnsurePrivateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Fatal(info.Mode())
	}
}
