package mcp

import (
	_ "embed"
	"encoding/json"
)

// ParamDoc documents one advanced Telegram parameter.
type ParamDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// MethodDoc documents advanced parameters of a Telegram Bot API method
// (Bot API 10.3). Common parameters are documented inline in tool schemas.
type MethodDoc struct {
	Method         string     `json:"method"`
	MCPTool        string     `json:"mcpTool"`
	Description    string     `json:"description"`
	AdvancedParams []ParamDoc `json:"advancedParams"`
}

//go:embed docs.json
var docsJSON []byte

var (
	docsByMethod = map[string]MethodDoc{}
	typeToMethod = map[string]string{}
	// KnownQueries lists every accepted get_telegram_parameter_doc query.
	KnownQueries []string
)

func init() {
	var raw struct {
		Docs  []MethodDoc `json:"docs"`
		Types [][2]string `json:"types"`
	}
	if err := json.Unmarshal(docsJSON, &raw); err != nil {
		panic("telesend: invalid embedded docs: " + err.Error())
	}
	for _, d := range raw.Docs {
		docsByMethod[d.Method] = d
		KnownQueries = append(KnownQueries, d.Method)
	}
	for _, t := range raw.Types {
		typeToMethod[t[0]] = t[1]
		KnownQueries = append(KnownQueries, t[0])
	}
}

// MethodDocFor looks up docs by Telegram method name ("sendMessage") or
// Telesend message type ("text", "video_note").
func MethodDocFor(query string) (MethodDoc, bool) {
	if d, ok := docsByMethod[query]; ok {
		return d, true
	}
	if m, ok := typeToMethod[query]; ok {
		d, ok := docsByMethod[m]
		return d, ok
	}
	return MethodDoc{}, false
}
