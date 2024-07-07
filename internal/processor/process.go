package processor

import (
	"fmt"
	"log"

	"gitlab.com/gomidi/midi/v2/sequencer"
	"gitlab.com/gomidi/midi/v2/smf"
)

type Pos struct {
	Bar int
	Pos uint8 // In 32ths.
}

type Range struct {
	Begin Pos
	End   Pos
}

// DumpTimeSig prints the time signatures the song uses in concise form.
func DumpTimeSig(song *sequencer.Song) {
	var start int
	var sig *[2]uint8
	gotSig := func(bar int, barSig *[2]uint8) {
		if barSig == nil || sig == nil || *barSig != *sig {
			if bar != start {
				plural := "s"
				if bar-start == 1 {
					plural = ""
				}
				log.Printf("%d: %d bar%s of %d/%d", start, bar-start, plural, sig[0], sig[1])
			}
			start = bar
			sig = barSig
		}
	}
	for i, bar := range song.Bars() {
		gotSig(i, &bar.TimeSig)
	}
	gotSig(len(song.Bars()), nil)
	log.Printf("%d: end", len(song.Bars()))
}

// Process processes the given MIDI file and writes the result to out.
func Process(in, out string, preludeSections []Range, numVerses int) error {
	midi, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	song := sequencer.FromSMF(*midi)
	DumpTimeSig(song)
	// Build new bars:
	// - preludeSections
	// - then the input numVerses times
	// Then write to output.
	return nil
}
