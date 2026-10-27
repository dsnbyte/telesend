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
