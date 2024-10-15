//go:build embed

package main

import (
	"embed"
	"io/fs"
)

//go:embed vfs/*
var vfs embed.FS

func openFS(pw string) (fs.FS, error) {
	return fs.Sub(vfs, "vfs")
}
