package file

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func ReadConfig(fsys fs.FS, configFile string) (*processor.Config, error) {
	f, err := fsys.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("could not open: %v", err)
	}
	defer f.Close()
	var config processor.Config
	err = yaml.NewDecoder(f).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not decode: %v", err)
	}
	return &config, nil
}
