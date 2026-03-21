// Package web embeds the compiled frontend assets into the binary.
package web

import "embed"

// FS contains the compiled Vite/React build output.
// Run `cd frontend && npm run build` to populate web/dist before compiling.
//
//go:embed dist
var FS embed.FS
