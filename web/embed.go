// Package web embeds the compiled frontend (built from ./frontend with `npm run build`).
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
