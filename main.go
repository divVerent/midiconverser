package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
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
	fermataExtend     = flag.Int("fermata_extend", 0, "fermata extension amount in beats (if negative, number of denominator notes)")
	fermataRest       = flag.Int("fermata_rest", 0, "fermata rest amount in beats (if negative, number of denominator notes)")
	restBetweenVerses = flag.Int("rest_between_verses", 0, "rest between verses in beats (if negative, number of denominator notes)")
	bpmOverride       = flag.Float64("bpm_override", 0, "when set, the new tempo to set")
	maxAdjust         = flag.Int64("max_adjust", 0, "maximum number of ticks to adjust cut boundaries by")
	holdRedundant     = flag.Bool("hold_redundant", false, "hold redundant notes (works better on some piano scores)")
)

var (
	preludeFlagValue = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?-(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?$`)
	fermataFlagValue = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?$`)
)

func parsePrelude(s string) []processor.Range {
	var ranges []processor.Range
	for _, item := range strings.Split(s, " ") {
		if item == "" {
			continue
		}
		result := preludeFlagValue.FindStringSubmatch(item)
		if result == nil {
			log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
		}
		r := processor.Range{
			Begin: processor.Pos{
				Beat:      1,
				BeatNum:   0,
				BeatDenom: 1,
			},
			End: processor.Pos{
				Beat:      1,
				BeatNum:   0,
				BeatDenom: 1,
			},
		}
		var err error
		r.Begin.Bar, err = strconv.Atoi(result[1])
		if err != nil {
			log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
		}
		if result[2] != "" {
			r.Begin.Beat, err = strconv.Atoi(result[2])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[3] != "" {
			r.Begin.BeatNum, err = strconv.Atoi(result[3])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[4] != "" {
			r.Begin.BeatDenom, err = strconv.Atoi(result[4])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		r.End.Bar, err = strconv.Atoi(result[5])
		if err != nil {
			log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
		}
		if result[6] != "" {
			r.End.Beat, err = strconv.Atoi(result[6])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[7] != "" {
			r.End.BeatNum, err = strconv.Atoi(result[7])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[8] != "" {
			r.End.BeatDenom, err = strconv.Atoi(result[8])
			if err != nil {
				log.Panicf("failed to parse --prelude: range %q not in format n.n+n/n-n.n+n/n", item)
			}
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
		result := fermataFlagValue.FindStringSubmatch(item)
		if result == nil {
			log.Panicf("failed to parse --fermatas: pos %q not in format n.n+n/n", item)
		}
		f := processor.Pos{
			Beat:      1,
			BeatNum:   0,
			BeatDenom: 1,
		}
		var err error
		f.Bar, err = strconv.Atoi(result[1])
		if err != nil {
			log.Panicf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
		if result[2] != "" {
			f.Beat, err = strconv.Atoi(result[2])
			if err != nil {
				log.Panicf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[3] != "" {
			f.BeatNum, err = strconv.Atoi(result[3])
			if err != nil {
				log.Panicf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		if result[4] != "" {
			f.BeatDenom, err = strconv.Atoi(result[4])
			if err != nil {
				log.Panicf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
			}
		}
		fermatas = append(fermatas, f)
	}
	return fermatas
}

func main() {
	flag.Parse()
	options := &processor.Options{
		InputFile:         *i,
		Fermatas:          parseFermatas(*fermatas),
		FermataExtend:     *fermataExtend,
		FermataRest:       *fermataRest,
		Prelude:           parsePrelude(*prelude),
		RestBetweenVerses: *restBetweenVerses,
		NumVerses:         *verses,
		BPMOverride:       *bpmOverride,
		MaxAdjust:         *maxAdjust,
		HoldRedundant:     *holdRedundant,
	}
	err := processor.Process(*o, *oPrefix, options)
	buf, err := json.MarshalIndent(options, "", "  ")
	fmt.Printf("%v\n", string(buf))
	if err != nil {
		log.Printf("Failed to process: %v", err)
		os.Exit(1)
	}
}
