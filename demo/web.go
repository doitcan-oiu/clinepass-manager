// Package web embeds the single-page frontend.
package web

import "embed"

//go:embed index.html settings.html static/*.js
var Files embed.FS
