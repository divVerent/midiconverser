package processor

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

func (p Pos) ToTick(b bars) int64 {
	return b.ToTick(p.Bar-1, p.Beat-1, p.BeatNum, p.BeatDenom)
}

type Range struct {
	Begin Pos
	End   Pos
}

func (r Range) ToTick(b bars) (int64, int64) {
	return r.Begin.ToTick(b), r.End.ToTick(b)
}

type tickRange struct {
	begin, end int64
}

func beatsOrNotesToTicks(b bar, n int) int64 {
	if n < 0 {
		// Negative: this uses denominator ticks.
		return -int64(n) * b.NumLength()
	} else {
		// Positive: this uses beats.
		return int64(n) * b.BeatLength()
	}
}

type tickFermata struct {
	tick   int64
	extend int64
	rest   int64
}

// Process processes the given MIDI file and writes the result to out.
func Process(in, out string, fermatas []Pos, fermataExtend, fermataRest int, preludeSections []Range, restBetweenVerses int, numVerses int) error {
	mid, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	bars := findBars(mid)
	log.Printf("bars: %+v", bars)
	dumpTimeSig("Before", bars)

	// Remove duplicate note start.
	removeRedundantNoteEvents(mid)

	// Map all to MIDI channel 2 for the organ.
	mapToChannel(mid, 1)

	// Fix overlapping notes.
	mergeOverlappingNotes(mid)

	// Convert all values to ticks.
	var fermataTick []tickFermata
	for _, f := range fermatas {
		fermataTick = append(fermataTick, tickFermata{
			tick:   f.ToTick(bars),
			extend: beatsOrNotesToTicks(bars[f.Bar], fermataExtend),
			rest:   beatsOrNotesToTicks(bars[f.Bar], fermataRest),
		})
	}
	var preludeTick []tickRange
	for _, p := range preludeSections {
		begin, end := p.ToTick(bars)
		preludeTick = append(preludeTick, tickRange{
			begin: begin,
			end:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], restBetweenVerses)
	fmt.Println(ticksBetweenVerses)

	return nil
	/*
		song = resequenceToOne(song, fermatas, preludeSections, restBetweenVerses, numVerses)
		song.SetBarAbsTicks()
		dumpTimeSig("After", song)
		// newMIDI := song.ToSMF1()
		newMIDI := song.ToSMF0().ConvertToSMF1()
		dumpSMF(newMIDI)
		return newMIDI.WriteFile(out)
	*/
}

// dumpTimeSig prints the time signatures the song uses in concise form.
func dumpTimeSig(prefix string, b bars) {
	var start int
	var startTicks int64
	var sigBar *bar
	gotSig := func(i int, thisBar *bar) {
		if thisBar == nil || sigBar == nil || thisBar.Num != sigBar.Num || thisBar.Denom != sigBar.Denom {
			if i != start {
				plural := "s"
				if i-start == 1 {
					plural = ""
				}
				log.Printf("%s: %d @ %d: %d bar%s of %d/%d", prefix, start, startTicks, i-start, plural, sigBar.Num, sigBar.Denom)
			}
			start = i
			if thisBar != nil {
				startTicks = thisBar.Start
			}
			sigBar = thisBar
		}
	}
	for i, bar := range b {
		gotSig(i, &bar)
	}
	gotSig(len(b), nil)
	log.Printf("%s: %d: end", prefix, len(b))
}

// computeDefaultRest computes the default rest between verses in beats.
func computeDefaultRest(b bars) int {
	if len(b) == 0 {
		return 1
	}
	lastBar := b[len(b)-1]
	log.Printf("last bar: %+v", lastBar)
	return 1
}

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(mid *smf.SMF, ch uint8) {
	for _, t := range mid.Tracks {
		for _, ev := range t {
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

func forEachEventWithTime(mid *smf.SMF, yield func(time int64, track int, msg smf.Message) error) error {
	// trackPos is the index of the NEXT event from each track.
	trackPos := make([]int, len(mid.Tracks))
	// trackTime is the time of the LAST event from each track.
	trackTime := make([]int64, len(mid.Tracks))
	for {
		earliestTrack := -1
		var earliestTime int64
		for i, t := range mid.Tracks {
			p := trackPos[i]
			if p >= len(t) {
				// End of track.
				continue
			}
			t := trackTime[i] + int64(t[p].Delta)
			if earliestTrack < 0 || t < earliestTime {
				earliestTime = t
				earliestTrack = i
			}
		}
		if earliestTrack < 0 {
			// End of MIDI.
			return nil
		}
		msg := mid.Tracks[earliestTrack][trackPos[earliestTrack]].Message
		if !msg.Is(smf.MetaEndOfTrackMsg) {
			err := yield(earliestTime, earliestTrack, mid.Tracks[earliestTrack][trackPos[earliestTrack]].Message)
			if err != nil {
				return err
			}
		}
		trackPos[earliestTrack]++
		trackTime[earliestTrack] = earliestTime
	}
}

type key struct {
	ch, note uint8
}

// removeRedundantNoteEvents removes overlapping note start events in the song.
func removeRedundantNoteEvents(mid *smf.SMF) error {
	notes := map[key]struct{}{}
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		var ch, note uint8
		if msg.GetNoteStart(&ch, &note, nil) {
			k := key{ch, note}
			if _, found := notes[k]; found {
				return nil
			}
			notes[k] = struct{}{}
		} else if msg.GetNoteEnd(&ch, &note) {
			k := key{ch, note}
			if _, found := notes[k]; !found {
				return nil
			}
			delete(notes, k)
		}
		tracks[track] = append(tracks[track], smf.Event{
			Delta:   uint32(time - trackTime[track]),
			Message: msg,
		})
		trackTime[track] = time
		return nil
	})
	if err != nil {
		return err
	}
	mid.Tracks = tracks
	return nil
}

// mergeOverlappingNotes merges overlapping notes in the song.
func mergeOverlappingNotes(mid *smf.SMF) error {
	notes := map[key]int{}
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		var ch, note uint8
		if msg.GetNoteStart(&ch, &note, nil) {
			k := key{ch, note}
			if notes[k]++; notes[k] != 1 {
				return nil
			}
		} else if msg.GetNoteEnd(&ch, &note) {
			k := key{ch, note}
			if notes[k]--; notes[k] != 0 {
				return nil
			}
		}
		tracks[track] = append(tracks[track], smf.Event{
			Delta:   uint32(time - trackTime[track]),
			Message: msg,
		})
		trackTime[track] = time
		return nil
	})
	if err != nil {
		return err
	}
	mid.Tracks = tracks
	return nil
}

type cut struct {
	restBefore int64
	tickRange
	restAfter int64
}

// cutMIDI generates a new MIDI file from the input and a set of ranges.
func cutMIDI(mid *smf.SMF, cuts []cut) (*smf.SMF, error) {
	var tracks []smf.Track
	var trackTimes []int64
	addEvent := func(t int, tick int64, msg smf.Message) {
		for t >= len(tracks) {
			tracks = append(tracks, nil)
			trackTimes = append(trackTimes, 0)
		}
		tracks[t] = append(tracks[t], smf.Event{
			Delta:   uint32(tick - trackTimes[t]),
			Message: msg,
		})
		trackTimes[t] = tick
	}
	closeTrack := func(t int, tick int64) {
		for t >= len(tracks) {
			tracks = append(tracks, nil)
			trackTimes = append(trackTimes, 0)
		}
		tracks[t].Close(uint32(tick - trackTimes[t]))
		trackTimes[t] = tick
	}
	copyMeta := func(from, to int64, outTick int64) error {
		return forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
			if time < from || time >= to {
				return nil
			}
			if msg.GetNoteStart(nil, nil, nil) {
				// Skip note starts.
				// We don't skip even note-off though! This serves to prevent surprises
				// in case any notes are left hanging while skipping forward.
				// Most of these will be handled by removeRedundantNoteEvents.
				return nil
			}
			addEvent(track, outTick, msg)
			return nil
		})
	}
	copyAll := func(from, to int64, outTick int64) error {
		return forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
			if time < from || time >= to {
				return nil
			}
			addEvent(track, outTick+time-from, msg)
			return nil
		})
	}

	prevEndTick := int64(0)
	outTick := int64(0)
	for _, cut := range cuts {
		// For each cut, all non-note events from the previous range's end to the next range's start are repeated.
		if prevEndTick > cut.begin {
			// If seeking backwards, we have to repeat events from the start.
			prevEndTick = 0
		}
		err := copyMeta(prevEndTick, cut.begin, outTick)
		if err != nil {
			return nil, err
		}
		outTick += cut.restBefore
		err = copyAll(cut.begin, cut.end, outTick)
		if err != nil {
			return nil, err
		}
		outTick += cut.end - cut.begin
		outTick += cut.restAfter
		prevEndTick = cut.end
	}
	for i := range tracks {
		closeTrack(i, outTick)
	}

	newMIDI := smf.NewSMF1()
	newMIDI.TimeFormat = mid.TimeFormat
	for _, t := range tracks {
		newMIDI.Add(t)
	}
	removeRedundantNoteEvents(newMIDI)
	return newMIDI, nil
}

/*
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

func dumpSMF(mid smf.SMF) {
	fmt.Printf("%v\n", mid)
}

func dumpSeq(song *sequencer.Song) {
	fmt.Printf("#### SEQ: %v\n", song)
	for i, bar := range song.bars() {
		fmt.Printf("## BAR %d: %v\n", i, bar)
	}
}
*/
