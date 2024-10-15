//go:build embed

package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io/fs"
)

//go:embed vfs.zip
var vfs []byte

func openFS(pw string) (fs.FS, error) {
	return zip.NewReader(bytes.NewReader(vfs), int64(len(vfs)))
}
