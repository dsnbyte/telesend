package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsnbyte/telesend/internal/apperr"
)

// FileConfig is the MCP local-file policy configuration.
type FileConfig struct {
	AllowPaths []string `json:"allow_paths"`
	Include    []string `json:"include"`
	Exclude    []string `json:"exclude"`
}

// Config is the on-disk Telesend configuration (config.json).
type Config struct {
	MCP struct {
		Files FileConfig `json:"files"`
	} `json:"mcp"`
}

// LoadConfig reads the config file at path. A missing file is only an error
// when explicit is true. Allowed paths must be absolute after ~ expansion.
func LoadConfig(path string, explicit bool, home string) (Config, error) {
	var cfg Config
	path, err := expandHome(path, home)
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return cfg, nil
		}
		if os.IsNotExist(err) {
			return cfg, apperr.Newf(apperr.Configuration, "MCP config file was not found: %s", path)
		}
		return cfg, apperr.Newf(apperr.Configuration, "Cannot read MCP config: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, apperr.Newf(apperr.Configuration, "Invalid MCP config: %v", err)
	}
	files := &cfg.MCP.Files
	for i, p := range files.AllowPaths {
		expanded, err := expandHome(p, home)
		if err != nil {
			return Config{}, err
		}
		if !filepath.IsAbs(expanded) {
			return Config{}, apperr.New(apperr.Configuration, "mcp.files.allow_paths entries must be absolute")
		}
		files.AllowPaths[i] = expanded
	}
	for _, list := range [][]string{files.AllowPaths, files.Include, files.Exclude} {
		for _, item := range list {
			if item == "" {
				return Config{}, apperr.New(apperr.Configuration, "mcp.files entries must be non-empty strings")
			}
		}
	}
	return cfg, nil
}

func expandHome(path, home string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home == "" {
			return "", apperr.New(apperr.Configuration, "HOME is required to expand MCP paths")
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
