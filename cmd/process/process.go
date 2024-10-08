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
	c           = flag.String("c", "midiconverser.yml", "config file name (YAML)")
	i           = flag.String("i", "", "input file name (YAML)")
	addChecksum = flag.Bool("add_checksum", false, "automatically add checksum to the input YAML")
	oPrefix     = flag.String("o_prefix", "", "output file name for outputting separate files")
)

func Main() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %v", err)
	}
	fsys := os.DirFS(cwd)

	config, err := file.ReadConfig(fsys, *c)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	options, err := file.ReadOptions(fsys, *i)
	if err != nil {
		return fmt.Errorf("failed to read options: %v", err)
	}

	wantChecksum := options.InputFileSHA256 == ""

	output, err := file.Process(fsys, config, options)
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
			return fmt.Errorf("failed to write %v: %v", name, err)
		}
	}

	if wantChecksum && options.InputFileSHA256 != "" {
		err := file.WriteOptions(*i, options)
		if err != nil {
			return fmt.Errorf("failed to write %v: %v", *i, err)
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
