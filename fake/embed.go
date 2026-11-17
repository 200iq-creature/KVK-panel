// Package fake holds the decoy website served on the VLESS auto-setup domains
// (Reality fallback + Cloudflare CDN front). Edit index.html here — it is
// embedded into the panel binary at build time. original/ keeps the source
// design it was derived from and is not served.
package fake

import "embed"

// FS contains the served files: index.html, 404.html, favicon.svg, robots.txt.
//
//go:embed index.html 404.html favicon.svg robots.txt
var FS embed.FS
