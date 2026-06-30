package debugfrontend

import (
	"embed"
	"io/fs"
)

//go:embed index.html index.css index.js
var files embed.FS

// FS returns the embedded debug frontend assets.
func FS() fs.FS {
	return files
}
