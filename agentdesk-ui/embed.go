package webassets

import "embed"

// Build with `vp run build` in this directory before building the Go binary.
//
//go:embed all:dist
var Files embed.FS
