package web

import "embed"

// Dist holds the built panel UI (frontend/ → web/dist).
//
//go:embed all:dist
var Dist embed.FS
