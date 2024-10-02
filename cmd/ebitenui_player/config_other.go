//go:build !wasm

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func loadConfigOverride(name string, into *processor.Config) error {
	return nil
}

func saveConfigOverride(name string, config *processor.Config) error {
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
