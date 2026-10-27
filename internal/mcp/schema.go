package mcp

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/dsnbyte/telesend/internal/apperr"
)

// M is shorthand for a JSON object.
type M = map[string]any

func str(desc string) M            { return M{"type": "string", "minLength": 1, "description": desc} }
func strAny(desc string) M         { return M{"type": "string", "description": desc} }
func integer(desc string) M        { return M{"type": "integer", "description": desc} }
func posInt(desc string) M         { return M{"type": "integer", "exclusiveMinimum": 0, "description": desc} }
func number(desc string) M         { return M{"type": "number", "description": desc} }
func boolean(desc string) M        { return M{"type": "boolean", "description": desc} }
func looseObj(desc string) M       { return M{"type": "object", "additionalProperties": true, "description": desc} }
func enum(desc string, v ...string) M { return M{"type": "string", "enum": v, "description": desc} }
func arr(items M, desc string) M   { return M{"type": "array", "items": items, "description": desc} }

func objectOf(props M, required ...string) M {
	s := M{"type": "object", "properties": props, "additionalProperties": true}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// validate checks v against the JSON Schema subset used by Telesend's tools:
// type, enum, minLength, exclusiveMinimum, properties, required, items.
func validate(schema M, v any, path string) error {
	bad := func(msg string) error { return apperr.Newf(apperr.Validation, "%s: %s", path, msg) }
	typ, _ := schema["type"].(string)
	switch typ {
	case "string":
		s, ok := v.(string)
		if !ok {
			return bad("expected string")
		}
		if min, _ := schema["minLength"].(int); min > 0 && len(s) < min {
			return bad("must not be empty")
		}
		if e, ok := schema["enum"].([]string); ok && !slices.Contains(e, s) {
			return bad("must be one of " + joinQuoted(e))
		}
	case "number", "integer":
		f, ok := asFloat(v)
		if !ok {
			return bad("expected " + typ)
		}
		if typ == "integer" && f != math.Trunc(f) {
			return bad("expected integer")
		}
		if min, ok := schema["exclusiveMinimum"].(int); ok && f <= float64(min) {
			return bad("must be greater than 0")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return bad("expected boolean")
		}
	case "array":
		items, ok := v.([]any)
		if !ok {
			return bad("expected array")
		}
		if sub, ok := schema["items"].(M); ok {
			for i, item := range items {
				if err := validate(sub, item, path+"["+itoa(i)+"]"); err != nil {
					return err
				}
			}
		}
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return bad("expected object")
		}
		if req, ok := schema["required"].([]string); ok {
			for _, k := range req {
				if _, present := obj[k]; !present {
					return apperr.Newf(apperr.Validation, "%s.%s: required", path, k)
				}
			}
		}
		if props, ok := schema["properties"].(M); ok {
			for k, sub := range props {
				if item, present := obj[k]; present {
					if err := validate(sub.(M), item, path+"."+k); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

func joinQuoted(values []string) string {
	out := ""
	for i, v := range values {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }
