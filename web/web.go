// Package web holds the dashboard UI embedded into the binary (ADR 20 §1):
// no CDN, no build step, works offline. server.go reads index.html and
// substitutes {{TOKEN}}; app.css and app.js are served as static files.
package web

import "embed"

// FS is the embedded UI.
//
//go:embed index.html app.css app.js
var FS embed.FS
