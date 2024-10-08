package file

import (
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func ReadOptions(fsys fs.FS, optionsFile string) (*processor.Options, error) {
	f, err := fsys.Open(optionsFile)
	if err != nil {
		return nil, fmt.Errorf("could not open: %v", err)
	}
	defer f.Close()
	var options processor.Options
	err = yaml.NewDecoder(f).Decode(&options)
	if err != nil {
		return nil, fmt.Errorf("could not decode: %v", err)
	}
	if options.InputFile == "" {
		return nil, fmt.Errorf("not a valid options file: no input file key")
	}
	return &options, nil
}

func WriteOptions(optionsFile string, options *processor.Options) (err error) {
	f, err := os.Create(optionsFile)
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
	return enc.Encode(options)
}
