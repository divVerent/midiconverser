package file

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func ReadConfig(configFile string) (*processor.Config, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("could not open %v: %v", configFile, err)
	}
	defer f.Close()
	var config processor.Config
	err = yaml.NewDecoder(f).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not decode %v: %v", configFile, err)
	}
	return &config, nil
}
