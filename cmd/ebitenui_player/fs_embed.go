//go:build embed

package main

import (
	"embed"
	"io/fs"
)

//go:embed vfs/*
var vfs embed.FS

func openFS() (fs.FS, error) {
	return fs.Sub(vfs, "vfs")
}
