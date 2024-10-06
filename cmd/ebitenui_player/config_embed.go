//go:build embed && !wasm

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
	return file.ReadConfig(fsys, *c)
}

func loadConfigOverride(name string, into *processor.Config) error {
	config, err := file.ReadConfig(os.DirFS("."), *c)
	if err != nil {
		return err
	}
	copyConfigOverrideFields(config, into)
	return nil
}

func saveConfigOverride(name string, config *processor.Config) (err error) {
	var subset processor.Config
	copyConfigOverrideFields(config, &subset)
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
	return enc.Encode(subset)
}
