//go:build !wasm && !embed

package main

import (
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

func loadConfig(fsys fs.FS, name string) (*processor.Config, error) {
	return file.ReadConfig(os.DirFS("."), *c)
}

func loadConfigOverride(name string, into *processor.Config) error {
	// Kinda superfluous, as loadConfig already got all, but it's needed for password.
	config, err := file.ReadConfig(os.DirFS("."), *c)
	if err != nil {
		return err
	}
	*into = *config
	return nil
}

func saveConfigOverride(name string, config *processor.Config) (err error) {
	// Actually save all fields here.
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
