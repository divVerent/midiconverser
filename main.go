package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gitlab.com/gomidi/midi/v2/smf"
	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/processor"
)

var (
	c           = flag.String("c", "config.yml", "config file name (YAML)")
	i           = flag.String("i", "", "input file name (YAML)")
	addChecksum = flag.Bool("add_checksum", false, "automatically add checksum to the input YAML")
	oPrefix     = flag.String("o_prefix", "", "output file name for outputting separate files")
)

func Main() (err error) {
	f, err := os.Open(*c)
	if err != nil {
		return fmt.Errorf("could not open %v: %v", *c, err)
	}
	defer f.Close()
	var config processor.Config
	err = yaml.NewDecoder(f).Decode(&config)
	if err != nil {
		return fmt.Errorf("could not decode %v: %v", *c, err)
	}

	f, err = os.Open(*i)
	// NOTE: Not deferring closing this file, as it may get reopened.
	if err != nil {
		return fmt.Errorf("could not open %v: %v", *i, err)
	}
	var options processor.Options
	err = yaml.NewDecoder(f).Decode(&options)
	if err != nil {
		f.Close()
		return fmt.Errorf("could not decode %v: %v", *i, err)
	}
	f.Close()

	if *oPrefix == "" {
		*oPrefix = strings.TrimSuffix(*i, ".yml")
	}

	inBytes, err := os.ReadFile(options.InputFile)
	if err != nil {
		return fmt.Errorf("could not read %v: %v", options.InputFile, err)
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(inBytes))

	if options.SHA256 != "" && options.SHA256 != sum {
		return fmt.Errorf("mismatching checksum of %v: got %v, want %v", options.InputFile, sum, options.SHA256)
	}

	in, err := smf.ReadFrom(bytes.NewReader(inBytes))
	if err != nil {
		return fmt.Errorf("could not parse %v: %v", options.InputFile, err)
	}

	output, err := processor.Process(in, &config, &options)
	if err != nil {
		return fmt.Errorf("Failed to process: %v", err)
	}

	for key, mid := range output {
		name := fmt.Sprintf("%s.%s.mid", *oPrefix, key)
		err := mid.WriteFile(name)
		if err != nil {
			return fmt.Errorf("Failed to write %v: %v", name, err)
		}
	}

	if options.SHA256 != "" {
		options.SHA256 = sum

		f, err = os.Create(*i)
		if err != nil {
			return fmt.Errorf("could not recreate %v: %v", *i, err)
		}
		defer func() {
			closeErr := f.Close()
			if closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		err := yaml.NewEncoder(f).Encode(options)
		if err != nil {
			return fmt.Errorf("could not encode %v: %v", *i, err)
		}
	}

	return nil
}

func main() {
	flag.Parse()
	err := Main()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
