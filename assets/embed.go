// Package assets embeds static files served by Telesend.
package assets

import _ "embed"

// Icon is the 512x512 Telesend icon (PNG).
//
//go:embed telesend-icon.png
var Icon []byte

// IconCompressed is the 200x200 Telesend icon (PNG) for inline base64 data URIs.
//
//go:embed telesend-icon-compressed.png
var IconCompressed []byte
