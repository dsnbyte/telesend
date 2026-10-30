// Package mcp implements Telesend's Model Context Protocol server: a small
// JSON-RPC 2.0 core, the tool definitions, and the local-file policy.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/jsonx"
)

// LatestProtocolVersion is the newest MCP revision implemented.
const LatestProtocolVersion = "2025-11-25"

var supportedVersions = []string{LatestProtocolVersion, "2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is one callable MCP tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	Schema      map[string]any
	Meta        map[string]any
	ReadOnly    bool
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// Server is a transport-independent MCP server.
type Server struct {
	Name, Version, Description, IconURL string
	tools                               []Tool
	index                               map[string]*Tool
}

// NewServer creates a server exposing tools.
func NewServer(name, version, description, iconURL string, tools []Tool) *Server {
	s := &Server{Name: name, Version: version, Description: description, IconURL: iconURL, tools: tools, index: map[string]*Tool{}}
	for i := range s.tools {
		s.index[s.tools[i].Name] = &s.tools[i]
	}
	return s
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Handle processes one JSON-RPC message and returns the encoded response, or
// nil when the message needs none (notifications and client responses).
func (s *Server) Handle(ctx context.Context, msg []byte) []byte {
	var req request
	if err := json.Unmarshal(msg, &req); err != nil {
		return encode(response{ID: json.RawMessage("null"), Error: &rpcError{codeParse, "Parse error"}})
	}
	if req.Method == "" || len(req.ID) == 0 {
		return nil // notification or response
	}
	result, rerr := s.dispatch(ctx, req)
	if rerr != nil {
		return encode(response{ID: req.ID, Error: rerr})
	}
	return encode(response{ID: req.ID, Result: result})
}

func encode(r response) []byte {
	r.JSONRPC = "2.0"
	if r.Error == nil && r.Result == nil {
		r.Result = struct{}{}
	}
	return jsonx.Marshal(r)
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := LatestProtocolVersion
		for _, v := range supportedVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		info := map[string]any{"name": s.Name, "version": s.Version, "description": s.Description}
		if s.IconURL != "" {
			info["icons"] = []any{
				map[string]any{
					"src":      s.IconURL,
					"mimeType": "image/png",
					"sizes":    []string{"512x512"},
				},
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      info,
		}, nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		list := make([]any, 0, len(s.tools))
		for _, t := range s.tools {
			annotations := map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": t.ReadOnly}
			if !t.ReadOnly {
				annotations["openWorldHint"] = true
			}
			toolDef := map[string]any{
				"name": t.Name, "title": t.Title, "description": t.Description,
				"inputSchema": t.Schema, "annotations": annotations,
			}
			if len(t.Meta) > 0 {
				toolDef["_meta"] = t.Meta
			}
			list = append(list, toolDef)
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "Invalid tools/call params"}
		}
		tool, ok := s.index[p.Name]
		if !ok {
			return nil, &rpcError{codeInvalidParams, "Tool " + p.Name + " not found"}
		}
		return s.call(ctx, tool, p.Arguments), nil
	}
	return nil, &rpcError{codeMethodNotFound, "Method not found: " + req.Method}
}

func (s *Server) call(ctx context.Context, tool *Tool, raw json.RawMessage) any {
	args := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		v, err := jsonx.DecodeBytes(raw)
		m, ok := v.(map[string]any)
		if err != nil || !ok {
			return toolError(apperr.New(apperr.Validation, "Tool arguments must be an object"))
		}
		args = m
	}
	if err := validate(tool.Schema, args, "arguments"); err != nil {
		return toolError(err)
	}
	data, err := tool.Handler(ctx, args)
	if err != nil {
		return toolError(err)
	}
	return map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": string(jsonx.Marshal(data))}},
		"structuredContent": data,
	}
}

func toolError(err error) any {
	e := apperr.From(err)
	return map[string]any{
		"isError": true,
		"content": []any{map[string]any{"type": "text", "text": string(e.Code) + ": " + e.Message}},
	}
}

// ServeStdio serves newline-delimited JSON-RPC on r and w until r closes.
// Only protocol frames are ever written to w.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var mu sync.Mutex
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			msg := append([]byte(nil), line...)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if out := s.Handle(ctx, msg); out != nil {
					mu.Lock()
					defer mu.Unlock()
					w.Write(append(out, '\n'))
				}
			}()
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
