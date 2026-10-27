// Package jsonx has small JSON helpers shared across packages.
package jsonx

import (
	"bytes"
	"encoding/json"
	"io"
)

// Marshal encodes v without HTML escaping or a trailing newline.
func Marshal(v any) []byte {
	return encode(v, "")
}

// MarshalIndent is Marshal with two-space indentation.
func MarshalIndent(v any) []byte {
	return encode(v, "  ")
}

func encode(v any, indent string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return []byte("null")
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// Decode reads one JSON value, keeping numbers exact as json.Number.
func Decode(r io.Reader) (any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeBytes decodes a JSON document held in memory.
func DecodeBytes(b []byte) (any, error) { return Decode(bytes.NewReader(b)) }
