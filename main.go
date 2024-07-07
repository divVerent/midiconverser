package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/divVerent/midiconverser/internal/processor"
)

var (
	i       = flag.String("i", "", "input file name")
	o       = flag.String("o", "", "output file name")
	prelude = flag.String("prelude", "", "prelude ranges of the form bar.beat-bar.beat bar.beat-bar.beat ...")
	verses  = flag.Int("verses", 1, "number of verses")
)

func parsePrelude(s string) []processor.Range {
	var ranges []processor.Range
	for _, item := range strings.Split(s, " ") {
		var r processor.Range
		_, err := fmt.Sscanf("%d.%d-%d.%d", item, &r.Begin.Bar, &r.Begin.Pos, &r.End.Bar, &r.End.Pos)
		if err != nil {
			log.Panicf("failed to parse --prelude: range %q not in format n.n-n.n", item)
		}
		ranges = append(ranges, r)
	}
	return ranges
}

func main() {
	flag.Parse()
	err := processor.Process(*i, *o, parsePrelude(*prelude), *verses)
	if err != nil {
		log.Printf("Failed to process: %v", err)
		os.Exit(1)
	}
}
