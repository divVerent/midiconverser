//go:build !wasm

package main

import (
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func openFS() (fs.FS, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %v", err)
	}
	fsys := os.DirFS(cwd)
	return fsys, nil
}

func loadConfigOverride(name string) (*processor.Config, error) {
	return nil, nil
}

func saveConfigOverride(name string, config *processor.Config) error {
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("could not recreate: %v", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2) // Match yq.
	return enc.Encode(config)
}
