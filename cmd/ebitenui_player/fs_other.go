//go:build !embed

package main

import (
	"io/fs"
	"os"
)

func openFS() (fs.FS, error) {
	return os.DirFS("."), nil
}
