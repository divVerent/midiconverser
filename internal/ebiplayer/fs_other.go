//go:build !embed

package ebiplayer

import (
	"io/fs"
	"os"
)

func openFS(pw string) (fs.FS, error) {
	return os.DirFS("."), nil
}
