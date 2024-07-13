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
	i                 = flag.String("i", "", "input file name")
	o                 = flag.String("o", "", "output file name for outputting a single file")
	oPrefix           = flag.String("o_prefix", "", "output file name for outputting separate files")
	prelude           = flag.String("prelude", "", "prelude ranges of the form bar.beat+num/denom-bar.beat+num/denom bar.beat+num/denom-bar.beat+num/denom ...")
	fermatas          = flag.String("fermatas", "", "fermata positions of the form bar.beat bar.beat ...")
	verses            = flag.Int("verses", 1, "number of verses")
	restBetweenVerses = flag.Int("rest_between_verses", 1, "rest between verses in beats (if negative, number of denominator notes)")
	fermataExtend     = flag.Int("fermata_extend", 1, "fermata extension amount in beats (if negative, number of denominator notes)")
	fermataRest       = flag.Int("fermata_rest", 1, "fermata rest amount in beats (if negative, number of denominator notes)")
)

func parsePrelude(s string) []processor.Range {
	var ranges []processor.Range
	for _, item := range strings.Split(s, " ") {
		if item == "" {
			continue
		}
		var r processor.Range
		_, err := fmt.Sscanf("%d.%d+%d/%d-%d.%d+%d/%d", item, &r.Begin.Bar, &r.Begin.Beat, &r.Begin.BeatNum, &r.Begin.BeatDenom, &r.End.Bar, &r.End.Beat, &r.End.BeatNum, &r.End.BeatDenom)
		if err != nil {
			log.Panicf("failed to parse --prelude: range %q not in format n.n-n.n", item)
		}
		ranges = append(ranges, r)
	}
	return ranges
}

func parseFermatas(s string) []processor.Pos {
	var fermatas []processor.Pos
	for _, item := range strings.Split(s, " ") {
		if item == "" {
			continue
		}
		var f processor.Pos
		_, err := fmt.Sscanf("%d.%d+%d/%d", item, &f.Bar, &f.Beat, &f.BeatNum, &f.BeatDenom)
		if err != nil {
			log.Panicf("failed to parse --fermatas: pos %q not in format n.n", item)
		}
		fermatas = append(fermatas, f)
	}
	return fermatas
}

func main() {
	flag.Parse()
	err := processor.Process(*i, *o, *oPrefix, parseFermatas(*fermatas), *fermataExtend, *fermataRest, parsePrelude(*prelude), *restBetweenVerses, *verses)
	if err != nil {
		log.Printf("Failed to process: %v", err)
		os.Exit(1)
	}
}
