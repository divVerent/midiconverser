package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/divVerent/midiconverser/internal/file"
)

var (
	c           = flag.String("c", "config.yml", "config file name (YAML)")
	i           = flag.String("i", "", "input file name (YAML)")
	addChecksum = flag.Bool("add_checksum", false, "automatically add checksum to the input YAML")
	oPrefix     = flag.String("o_prefix", "", "output file name for outputting separate files")
)

func Main() (err error) {
	output, err := file.Process(*c, *i, *addChecksum)
	if err != nil {
		return fmt.Errorf("failed to process: %v", err)
	}

	if *oPrefix == "" {
		*oPrefix = strings.TrimSuffix(*i, ".yml")
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
