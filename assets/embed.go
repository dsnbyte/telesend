// Package assets embeds static files served by Telesend.
package assets

import _ "embed"

// Icon is the 200x200 Telesend icon (PNG).
//
//go:embed telesend-icon-compressed.png
var Icon []byte
