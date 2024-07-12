package processor

// TODO: Rewrite all without sequencer, as sequencer seems VERY broken.
// Thus, make our own absolute-time structure, then work in there.
// Can retain track structure by remembering which event was on which track.
// Hardest part is that we need bar/beat number <-> abstime mapping.

import (
	"fmt"
	"log"

	"gitlab.com/gomidi/midi/v2/smf"
)

type Pos struct {
	Bar       int
	Beat      int
	BeatNum   int
	BeatDenom int
}

type Range struct {
	Begin Pos
	End   Pos
}

// Process processes the given MIDI file and writes the result to out.
func Process(in, out string, fermatas []Pos, preludeSections []Range, restBetweenVerses int8, numVerses int) error {
	midi, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	bars := findBars(midi)
	log.Printf("bars: %+v", bars)
	return nil
	/*
		dumpSMF(*midi)
		dumpTimeSig("Before", song)
		mapToChannel(song, 1) // Map to MIDI channel 2.
		mergeOverlappingNotes(song)
		if restBetweenVerses < 0 {
			restBetweenVerses = computeDefaultRest(song)
			log.Printf("computed default rest between verses: %d/32", restBetweenVerses)
		}
		song = resequenceToOne(song, fermatas, preludeSections, restBetweenVerses, numVerses)
		song.SetBarAbsTicks()
		dumpTimeSig("After", song)
		// newMIDI := song.ToSMF1()
		newMIDI := song.ToSMF0().ConvertToSMF1()
		dumpSMF(newMIDI)
		return newMIDI.WriteFile(out)
	*/
}

/*
// dumpTimeSig prints the time signatures the song uses in concise form.
func dumpTimeSig(prefix string, song *sequencer.Song) {
	var start int
	var startTicks int64
	var sig *[2]uint8
	gotSig := func(bar int, absTicks int64, barLen uint8, barSig *[2]uint8) {
		if barSig == nil || sig == nil || *barSig != *sig {
			if bar != start {
				plural := "s"
				if bar-start == 1 {
					plural = ""
				}
				log.Printf("%s: %d @ %d: %d bar%s of %d/%d", prefix, start, startTicks, bar-start, plural, sig[0], sig[1])
			}
			start = bar
			startTicks = absTicks
			sig = barSig
		}
	}
	for i, bar := range song.bars() {
		log.Printf("bar %d ticks %d", i, bar.AbsTicks)
		gotSig(i, bar.AbsTicks, bar.Len(), &bar.TimeSig)
	}
	gotSig(len(song.bars()), -1, 0, nil)
	log.Printf("%s: %d: end", prefix, len(song.bars()))
}

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(song *sequencer.Song, ch uint8) {
	for _, bar := range song.bars() {
		for _, ev := range bar.Events {
			var evCh uint8
			if ev.Message.GetChannel(&evCh) {
				ev.Message.Bytes()[0] += ch - evCh
			}
			if ev.Message.GetMetaChannel(&evCh) {
				ev.Message.Bytes()[3] += ch - evCh
			}
		}
	}
}

// mergeOverlappingNotes merges overlapping notes in the song.
func mergeOverlappingNotes(song *sequencer.Song) error {
	type key struct {
		ch, note uint8
	}
	type item struct {
		start, end int64
		velocity   uint8
		erase      func()
		deleted    bool
	}
	notes := map[key][]item{}
	handleNoteOn := func(i int, k key, velocity uint8, start, end int64, erase func()) (int64, int64) {
		items := notes[k]
		for i, item := range items {
			if item.deleted {
				continue
			}
			if item.start >= end {
				continue
			}
			if item.end <= start {
				continue
			}
			if item.start < start {
				start = item.start
			}
			if item.end > end {
				end = item.end
			}
			if item.velocity > velocity {
				velocity = item.velocity
			}
			item.erase()
			items[i].deleted = true
			log.Printf("eliminated overlapping note %v in bar %v at tick %d", k, i, item.start)
		}
		notes[k] = append(notes[k], item{
			start:    start,
			end:      end,
			velocity: velocity,
			erase:    erase,
			deleted:  false,
		})
		return start, end
	}
	for i, bar := range song.bars() {
		for _, ev := range bar.Events {
			start, end := ev.AbsTicks(bar, song.Ticks)
			var ch, note, velocity uint8
			if ev.Message.GetNoteOn(&ch, &note, &velocity) {
				newStart, newEnd := handleNoteOn(i, key{ch, note}, velocity, start, end, func() {
					// To be removed later.
					ev.Message = nil
				})
				newBar := song.FindBar(newStart)
				if newBar != bar {
					return fmt.Errorf("tried to coalesce notes across bars in bar %d event %v", i, ev)
				}
				ev.Pos = uint8((newStart - bar.AbsTicks) / int64(song.Ticks.Ticks32th()))
				ev.Duration = uint8((newEnd - newStart) / int64(song.Ticks.Ticks32th()))
			}
		}
	}
	// Filter out all events with nil message.
	for _, bar := range song.bars() {
		var newEvents sequencer.Events
		for _, ev := range bar.Events {
			if ev.Message != nil {
				newEvents = append(newEvents, ev)
			}
		}
		bar.Events = newEvents
		bar.SortEvents()
	}
	return nil
}

// computeDefaultRest computes the default rest between verses.
func computeDefaultRest(song *sequencer.Song) int8 {
	if len(song.bars()) == 0 {
		return 8 // Default: 1/4 note.
	}
	lastBar := song.bars()[len(song.bars())-1]
	timeSig := lastBar.TimeSig
	if timeSig[0] > 3 && timeSig[0]%3 == 0 {
		// Multiple of 3: then 3 form a group.
		return int8(3 * 32 / timeSig[1])
	}
	// Otherwise just one beat.
	return int8(32 / timeSig[1])
}

// resequenceToOne resequences the song.
func resequenceToOne(song *sequencer.Song, fermatas []Pos, preludeSections []Range, restBetweenVerses int8, numVerses int) *sequencer.Song {
	newSong := sequencer.New()
	newSong.Title = song.Title
	newSong.Composer = song.Composer
	newSong.TrackNames = song.TrackNames
	newSong.Ticks = song.Ticks

	if len(fermatas) > 0 {
		log.Printf("NOT YET IMPLEMENTED: fermatas")
	}

	if len(preludeSections) != 0 {
		log.Printf("NOT YET IMPLEMENTED: prelude")
	}

	restSig := [2]uint8{
		uint8(restBetweenVerses),
		32,
	}
	for restSig[0] >= 8 || (restSig[0] > 1 && restSig[0]%2 == 0) {
		restSig[0] /= 2
		restSig[1] /= 2
	}

	for i := 0; i < numVerses; i++ {
		if i > 0 {
			newBar := sequencer.bar{
				TimeSig: restSig,
				Events:  sequencer.Events{},
				Key:     nil,
			}
			newSong.AddBar(newBar)
		}
		for _, bar := range song.bars() {
			newSong.AddBar(*bar)
		}
	}

	newSong.SetBarAbsTicks()
	return newSong
}

// resequenceToMultiple resequences the song to multiple separate MIDI files that are played in sequence.
// Output files are in order:
// - Prelude (if any).
// - Then one file per segment, split at fermatas.
func resequenceToMultiple(song *sequencer.Song, fermatas []Pos, preludeSections []Range) (*sequencer.Song, []*sequencer.Song) {
	log.Printf("NOT YET IMPLEMENTED: resequenceToMultiple")
	return nil, nil
}

func dumpSMF(midi smf.SMF) {
	fmt.Printf("%v\n", midi)
}

func dumpSeq(song *sequencer.Song) {
	fmt.Printf("#### SEQ: %v\n", song)
	for i, bar := range song.bars() {
		fmt.Printf("## BAR %d: %v\n", i, bar)
	}
}
*/
