package file

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"gitlab.com/gomidi/midi/v2/smf"
	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

func Process(configFile, optionsFile string, addChecksum bool) (map[processor.OutputKey]*smf.SMF, error) {
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

	f, err = os.Open(optionsFile)
	// NOTE: Not deferring closing this file, as it may get reopened.
	if err != nil {
		return nil, fmt.Errorf("could not open %v: %v", optionsFile, err)
	}
	var options processor.Options
	err = yaml.NewDecoder(f).Decode(&options)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("could not decode %v: %v", optionsFile, err)
	}
	f.Close()

	inBytes, err := os.ReadFile(options.InputFile)
	if err != nil {
		return nil, fmt.Errorf("could not read %v: %v", options.InputFile, err)
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(inBytes))

	if options.InputFileSHA256 != "" && options.InputFileSHA256 != sum {
		return nil, fmt.Errorf("mismatching checksum of %v: got %v, want %v", options.InputFile, sum, options.InputFileSHA256)
	}

	in, err := smf.ReadFrom(bytes.NewReader(inBytes))
	if err != nil {
		return nil, fmt.Errorf("could not parse %v: %v", options.InputFile, err)
	}

	output, err := processor.Process(in, &config, &options)
	if err != nil {
		return nil, fmt.Errorf("failed to process: %v", err)
	}

	if options.InputFileSHA256 == "" && addChecksum {
		options.InputFileSHA256 = sum

		f, err = os.Create(optionsFile)
		if err != nil {
			return nil, fmt.Errorf("could not recreate %v: %v", optionsFile, err)
		}
		defer func() {
			closeErr := f.Close()
			if closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		enc := yaml.NewEncoder(f)
		enc.SetIndent(2) // Match yq.
		err := enc.Encode(options)
		if err != nil {
			return nil, fmt.Errorf("could not encode %v: %v", optionsFile, err)
		}
	}

	return output, nil
}
