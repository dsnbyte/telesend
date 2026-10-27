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

// Default resolves paths for the current process.
func Default() (Paths, error) { return Resolve(os.Getenv, runtime.GOOS) }

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
