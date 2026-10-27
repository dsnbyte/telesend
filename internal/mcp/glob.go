package mcp

import (
	"regexp"
	"strings"
)

// Glob is a compiled glob pattern matched against slash-separated paths.
// Supported syntax: *, **, ?, {a,b} alternation, and [abc] classes.
type Glob struct{ re *regexp.Regexp }

// CompileGlob compiles pattern.
func CompileGlob(pattern string) (*Glob, error) {
	var b strings.Builder
	b.WriteString("^")
	depth := 0
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '*' && strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case c == '/' && strings.HasPrefix(pattern[i:], "/**") && i+3 == len(pattern):
			b.WriteString("/.*")
			i += 2
		case c == '*' && strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			depth++
			b.WriteString("(?:")
		case c == '}' && depth > 0:
			depth--
			b.WriteString(")")
		case c == ',' && depth > 0:
			b.WriteString("|")
		case c == '[':
			end := strings.IndexByte(pattern[i:], ']')
			if end < 0 {
				b.WriteString(regexp.QuoteMeta("["))
				continue
			}
			class := pattern[i+1 : i+end]
			class = strings.TrimPrefix(strings.Replace(class, "\\", "\\\\", -1), "!")
			if strings.HasPrefix(pattern[i+1:], "!") {
				class = "^" + class
			}
			b.WriteString("[" + class + "]")
			i += end
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	return &Glob{re}, nil
}

// Match reports whether path matches the pattern.
func (g *Glob) Match(path string) bool { return g.re.MatchString(path) }
