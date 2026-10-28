// Package paths resolves Telesend data and config locations.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Paths are the resolved Telesend locations.
type Paths struct {
	ConfigDir    string
	ConfigFile   string
	DataDir      string
	DatabaseFile string
}

// Resolve computes paths for goos using getenv.
func Resolve(getenv func(string) string, goos string) (Paths, error) {
	home := getenv("HOME")
	if home == "" && goos != "windows" {
		return Paths{}, errors.New("HOME is required when XDG paths are not configured")
	}
	or := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}
	var dataRoot, configRoot string
	switch goos {
	case "windows":
		dataRoot = or("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
		configRoot = or("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	case "darwin":
		dataRoot = or("XDG_DATA_HOME", filepath.Join(home, "Library", "Application Support"))
		configRoot = or("XDG_CONFIG_HOME", filepath.Join(home, "Library", "Preferences"))
	default:
		dataRoot = or("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
		configRoot = or("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	}
	dataDir := filepath.Join(dataRoot, "telesend")
	configDir := filepath.Join(configRoot, "telesend")
	return Paths{
		ConfigDir:    configDir,
		ConfigFile:   filepath.Join(configDir, "config.json"),
		DataDir:      dataDir,
		DatabaseFile: filepath.Join(dataDir, "telesend.db"),
	}, nil
}

// ExecutableDir returns the directory containing the running executable.
// If the executable is a symlink, it resolves the symlink to the real target directory.
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = realPath
	}
	return filepath.Dir(exe), nil
}

// ForDir computes Paths given a data directory.
func ForDir(dataDir string) Paths {
	return Paths{
		ConfigDir:    dataDir,
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		DataDir:      dataDir,
		DatabaseFile: filepath.Join(dataDir, "telesend.db"),
	}
}

// Default resolves paths for the current process.
// By default, it places the data directory in the "data" folder alongside
// the executable (similar to PocketBase).
// TELESEND_DATA_DIR can be set to override this location.
func Default() (Paths, error) {
	if envDir := os.Getenv("TELESEND_DATA_DIR"); envDir != "" {
		return ForDir(envDir), nil
	}
	exeDir, err := ExecutableDir()
	if err != nil {
		exeDir = "."
	}
	dataDir := filepath.Join(exeDir, "data")
	p := ForDir(dataDir)
	if _, err := os.Stat(filepath.Join(exeDir, "config.json")); err == nil {
		p.ConfigFile = filepath.Join(exeDir, "config.json")
	}
	return p, nil
}

// EnsurePrivateDir creates path with mode 0700.
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0o700)
	}
	return nil
}
