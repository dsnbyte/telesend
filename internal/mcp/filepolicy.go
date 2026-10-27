package mcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dsnbyte/telesend/internal/apperr"
)

// ProtectedExclusions can never be overridden.
var ProtectedExclusions = []string{
	"**/.env",
	"**/.env.*",
	"**/*.{pem,key,p12,pfx}",
	"**/.git",
	"**/.git/**",
}

// DefaultExclusions are denied unless a user include pattern matches.
var DefaultExclusions = []string{
	"**/*.{ts,tsx,js,jsx,mjs,cjs,mts,cts}",
	"**/*.{py,go,rs,java,kt,kts,rb,php,c,h,cpp,hpp,cs,swift,scala}",
	"**/*.{sh,bash,zsh,fish,ps1,sql}",
	"**/*.{toml,yaml,yml,ini,conf}",
	"**/package.json",
	"**/tsconfig*.json",
	"**/jsconfig*.json",
	"**/go.mod",
	"**/go.sum",
	"**/bun.lock",
	"**/bun.lockb",
	"**/src/**",
	"**/source/**",
	"**/config/**",
	"**/configs/**",
	"**/assets/**",
	"**/node_modules/**",
	"**/vendor/**",
}

// FilePolicyOptions configures NewFilePolicy.
type FilePolicyOptions struct {
	CWD             string
	ConfiguredRoots []string
	CLIRoots        []string
	Include         []string
	Exclude         []string
	ProtectedPaths  []string
}

// FilePolicy authorizes local files for MCP tools. It fails closed.
type FilePolicy struct {
	roots                                       []string
	includes, excludes, protected, defaultGlobs []*Glob
	protectedPaths                              map[string]bool
}

func compileAll(patterns []string) ([]*Glob, error) {
	out := make([]*Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := CompileGlob(p)
		if err != nil {
			return nil, apperr.Newf(apperr.Configuration, "Invalid glob pattern %q", p)
		}
		out = append(out, g)
	}
	return out, nil
}

// NewFilePolicy builds a policy rooted at the working directory plus any
// configured and command-line roots.
func NewFilePolicy(o FilePolicyOptions) (*FilePolicy, error) {
	raw := append([]string{o.CWD}, o.ConfiguredRoots...)
	for _, p := range o.CLIRoots {
		if !filepath.IsAbs(p) {
			p = filepath.Join(o.CWD, p)
		}
		raw = append(raw, p)
	}
	p := &FilePolicy{protectedPaths: map[string]bool{}}
	seen := map[string]bool{}
	for _, r := range raw {
		canonical, err := filepath.EvalSymlinks(r)
		if err == nil {
			var info os.FileInfo
			if info, err = os.Stat(canonical); err == nil && !info.IsDir() {
				err = os.ErrInvalid
			}
		}
		if err != nil {
			return nil, apperr.Newf(apperr.Configuration, "MCP allowed path is not a directory: %s", r)
		}
		if !seen[canonical] {
			seen[canonical] = true
			p.roots = append(p.roots, canonical)
		}
	}
	for _, path := range o.ProtectedPaths {
		if c, err := filepath.EvalSymlinks(path); err == nil {
			p.protectedPaths[c] = true
		} else if abs, err := filepath.Abs(path); err == nil {
			p.protectedPaths[abs] = true
		}
	}
	var err error
	if p.includes, err = compileAll(o.Include); err != nil {
		return nil, err
	}
	if p.excludes, err = compileAll(o.Exclude); err != nil {
		return nil, err
	}
	p.protected, _ = compileAll(ProtectedExclusions)
	p.defaultGlobs, _ = compileAll(DefaultExclusions)
	return p, nil
}

func matchAny(globs []*Glob, path string) bool {
	for _, g := range globs {
		if g.Match(path) {
			return true
		}
	}
	return false
}

// Authorize returns the canonical path of file when policy allows reading it.
func (p *FilePolicy) Authorize(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err == nil {
		canonical, err = filepath.Abs(canonical)
	}
	if err != nil {
		return "", apperr.Newf(apperr.FilesystemPolicy, "Local file does not exist: %s", path)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() {
		return "", apperr.Newf(apperr.FilesystemPolicy, "Local path is not a regular file: %s", path)
	}
	f, err := os.Open(canonical)
	if err != nil {
		return "", apperr.Newf(apperr.FilesystemPolicy, "Local file is not readable: %s", path)
	}
	f.Close()

	var rel string
	found := false
	for _, root := range p.roots {
		if r, err := filepath.Rel(root, canonical); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r) {
			rel, found = filepath.ToSlash(r), true
			break
		}
	}
	switch {
	case !found:
		return "", apperr.New(apperr.FilesystemPolicy, "Local file is outside the allowed MCP paths")
	case p.protectedPaths[canonical] || matchAny(p.protected, rel):
		return "", apperr.New(apperr.FilesystemPolicy, "Local file is protected by MCP policy")
	case matchAny(p.excludes, rel):
		return "", apperr.New(apperr.FilesystemPolicy, "Local file matches an MCP exclude pattern")
	case matchAny(p.defaultGlobs, rel) && !matchAny(p.includes, rel):
		return "", apperr.New(apperr.FilesystemPolicy, "Local file matches a default MCP exclusion")
	}
	return canonical, nil
}
