//go:build !wasm

package main

import (
	"fmt"
	"io/fs"
	"os"
)

func openFS() (fs.FS, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %v", err)
	}
	fsys := os.DirFS(cwd)
	return fsys, nil
}
