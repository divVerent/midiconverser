package main

import (
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
	c       = flag.String("c", "config.yml", "config file name (YAML)")
	i       = flag.String("i", "", "input file name (YAML)")
	oPrefix = flag.String("o_prefix", "", "output file name for outputting separate files")
)

func Main() error {
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
	if err != nil {
		return fmt.Errorf("could not open %v: %v", *i, err)
	}
	defer f.Close()
	var options processor.Options
	err = yaml.NewDecoder(f).Decode(&options)
	if err != nil {
		return fmt.Errorf("could not decode %v: %v", *i, err)
	}

	if *oPrefix == "" {
		*oPrefix = strings.TrimSuffix(*i, ".yml")
	}

	in, err := smf.ReadFile(options.InputFile)
	if err != nil {
		return fmt.Errorf("could not read %v: %v", options.InputFile, err)
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
