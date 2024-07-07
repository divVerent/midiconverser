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

// Process processes the given MIDI file and writes the result to out.
func Process(in, out string, fermatas []Pos, preludeSections []Range, restBetweenVerses int8, numVerses int) error {
	midi, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	song := sequencer.FromSMF(*midi)
	dumpTimeSig("Before", song)
	mapToChannel(song, 1) // Map to MIDI channel 2.
	mergeOverlappingNotes(song)
	addFermatas(song, fermatas)
	if restBetweenVerses < 0 {
		restBetweenVerses = computeDefaultRest(song)
		log.Printf("computed default rest between verses: %d/32", restBetweenVerses)
	}
	song = resequence(song, preludeSections, restBetweenVerses, numVerses)
	dumpTimeSig("After", song)
	newMIDI := song.ToSMF1()
	return newMIDI.WriteFile(out)
}

// dumpTimeSig prints the time signatures the song uses in concise form.
func dumpTimeSig(prefix string, song *sequencer.Song) {
	var start int
	var sig *[2]uint8
	gotSig := func(bar int, barSig *[2]uint8) {
		if barSig == nil || sig == nil || *barSig != *sig {
			if bar != start {
				plural := "s"
				if bar-start == 1 {
					plural = ""
				}
				log.Printf("%s: %d: %d bar%s of %d/%d", prefix, start, bar-start, plural, sig[0], sig[1])
			}
			start = bar
			sig = barSig
		}
	}
	for i, bar := range song.Bars() {
		gotSig(i, &bar.TimeSig)
	}
	gotSig(len(song.Bars()), nil)
	log.Printf("%s: %d: end", prefix, len(song.Bars()))
}

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(song *sequencer.Song, ch uint8) {
	for _, bar := range song.Bars() {
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
	for i, bar := range song.Bars() {
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
	for _, bar := range song.Bars() {
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

// addFermatas performs fermatas in the song.
func addFermatas(song *sequencer.Song, fermatas []Pos) {
	if len(fermatas) > 0 {
		log.Printf("NOT YET IMPLEMENTED: fermatas")
	}
}

// computeDefaultRest computes the default rest between verses.
func computeDefaultRest(song *sequencer.Song) int8 {
	if len(song.Bars()) == 0 {
		return 8 // Default: 1/4 note.
	}
	lastBar := song.Bars()[len(song.Bars())-1]
	timeSig := lastBar.TimeSig
	if timeSig[0] > 3 && timeSig[0]%3 == 0 {
		// Multiple of 3: then 3 form a group.
		return int8(3 * 32 / timeSig[1])
	}
	// Otherwise just one beat.
	return int8(32 / timeSig[1])
}

// resequence resequences the song.
func resequence(song *sequencer.Song, preludeSections []Range, restBetweenVerses int8, numVerses int) *sequencer.Song {
	newSong := sequencer.New()
	newSong.Title = song.Title
	newSong.Composer = song.Composer
	newSong.TrackNames = song.TrackNames
	newSong.Ticks = song.Ticks

	if len(preludeSections) != 0 {
		log.Printf("NOT YET IMPLEMENTED: prelude")
	}

	restSig := [2]uint8{
		uint8(restBetweenVerses),
		32,
	}

	for i := 0; i < numVerses; i++ {
		if i > 0 {
			newBar := sequencer.Bar{
				TimeSig: restSig,
				Events:  nil,
				Key:     nil,
			}
			newSong.AddBar(newBar)
		}
		for _, bar := range song.Bars() {
			newSong.AddBar(*bar)
		}
	}

	newSong.SetBarAbsTicks()
	return newSong
}
