package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distEmbed embed.FS

// DistFS returns the embedded dist filesystem
func DistFS() (fs.FS, error) {
	return fs.Sub(distEmbed, "dist")
}
