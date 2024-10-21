//go:build wasm

package ebiplayer

import (
	"bytes"
	"fmt"
	"io/fs"
	"syscall/js"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

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

func loadConfig(fsys fs.FS, name string) (*processor.Config, error) {
	return file.ReadConfig(fsys, name)
}

func loadConfigOverride(name string, into *processor.Config) error {
	var data js.Value
	err := protectJS(func() {
		data = js.Global().Get("localStorage").Call("getItem", "midiconverser.yml")
	})
	if err != nil {
		return err
	}
	if data.IsNull() {
		return nil
	}
	if data.Type() != js.TypeString {
		return fmt.Errorf("unexpected localStorage type for midiconverser.yml: got %v, want string", data.Type())
	}
	buf := bytes.NewReader([]byte(data.String()))
	var config processor.Config
	err = yaml.NewDecoder(buf).Decode(&config)
	if err != nil {
		return fmt.Errorf("could not decode: %v", err)
	}
	copyConfigOverrideFields(&config, into)
	return nil
}

func saveConfigOverride(name string, config *processor.Config) error {
	var subset processor.Config
	copyConfigOverrideFields(config, &subset)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // Match yq.
	err := enc.Encode(subset)
	if err != nil {
		return err
	}
	return protectJS(func() {
		js.Global().Get("localStorage").Call("setItem", "midiconverser.yml", buf.String())
	})
}
