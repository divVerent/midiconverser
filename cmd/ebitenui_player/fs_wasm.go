//go:build wasm

package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"syscall/js"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

//go:embed vfs/*
var vfs embed.FS

func openFS() (fs.FS, error) {
	return fs.Sub(vfs, "vfs")
}

func protectJS(f func()) (err error) {
	ok := false
	defer func() {
		if !ok {
			err = fmt.Errorf("caught JS exception: %v", recover())
		}
	}()
	f()
	ok = true
	return
}

func loadConfigOverride(name string) (*processor.Config, error) {
	var data js.Value
	err := protectJS(func() {
		data = js.Global().Get("localStorage").Call("getItem", "midiconverser.yml")
	})
	if err != nil {
		return nil, err
	}
	if data.IsNull() {
		return nil, nil
	}
	if data.Type() != js.TypeString {
		return nil, fmt.Errorf("unexpected localStorage type for midiconverser.yml: got %v, want string", data.Type())
	}
	buf := bytes.NewReader([]byte(data.String()))
	var config processor.Config
	err = yaml.NewDecoder(buf).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not decode: %v", err)
	}
	return &config, nil
}

func saveConfigOverride(name string, config *processor.Config) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // Match yq.
	err := enc.Encode(config)
	if err != nil {
		return err
	}
	return protectJS(func() {
		js.Global().Get("localStorage").Call("setItem", "midiconverser.yml", buf.String())
	})
}
