//go:build !embed

package main

import (
	"io/fs"
	"os"
)

func openFS(pw string) (fs.FS, error) {
	return os.DirFS("."), nil
}
