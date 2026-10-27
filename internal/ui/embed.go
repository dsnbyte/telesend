// Package ui embeds and serves the Telesend web interface.
package ui

import "embed"

//go:embed static/*
var staticFS embed.FS
