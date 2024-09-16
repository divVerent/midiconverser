package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/divVerent/midiconverser/internal/processor"
)

var (
	i       = flag.String("i", "", "input file name (JSON)")
	oPrefix = flag.String("o_prefix", "", "output file name for outputting separate files")
)

func main() {
	flag.Parse()
	f, err := os.Open(*i)
	if err != nil {
		log.Printf("could not open %v: %v", *i, err)
		os.Exit(1)
	}
	defer f.Close()
	var options processor.Options
	err = json.NewDecoder(f).Decode(&options)
	if err != nil {
		log.Printf("could not decode %v: %v", *i, err)
		os.Exit(1)
	}
	if *oPrefix == "" {
		*oPrefix = strings.TrimSuffix(*i, ".json")
	}
	err = processor.Process(*oPrefix, &options)
	if err != nil {
		log.Printf("Failed to process: %v", err)
		os.Exit(1)
	}
}
