package processor

import (
	"fmt"
	"log"

	"gitlab.com/gomidi/midi/v2"
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

	// Values computed from the inputs.
	holdTick    int64 // Last tick where all notes are held.
	releaseTick int64 // First tick with a note after the fermata; -1 indicates till end.
}

type Options struct {
	Fermatas          []Pos
	FermataExtend     int
	FermataRest       int
	Prelude           []Range
	RestBetweenVerses int
	NumVerses         int
	BPMOverride       float64
	MaxAdjust         int64

	// TODO: Option to sort all NoteOff events first in a tick.
	// Relaxes cutting locations, but MAY break things a bit.
	// Default on.
}

// Process processes the given MIDI file and writes the result to out.
func Process(in, out, outPrefix string, options *Options) error {
	mid, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	bars := findBars(mid)
	log.Printf("bars: %+v", bars)
	dumpTimeSig("Before", bars)

	// Fix bad events.
	err = removeUnneededEvents(mid)
	if err != nil {
		return err
	}

	// Remove duplicate note start.
	err = removeRedundantNoteEvents(mid, false)
	if err != nil {
		return err
	}

	// Map all to MIDI channel 2 for the organ.
	mapToChannel(mid, 1)
	if err != nil {
		return err
	}

	// Fix overlapping notes.
	err = removeRedundantNoteEvents(mid, true)
	if err != nil {
		return err
	}

	if options.BPMOverride > 0 {
		err = forceTempo(mid, options.BPMOverride)
	}

	// Convert all values to ticks.
	var fermataTick []tickFermata
	for _, f := range options.Fermatas {
		tf := tickFermata{
			tick:   f.ToTick(bars),
			extend: beatsOrNotesToTicks(bars[f.Bar-1], options.FermataExtend),
			rest:   beatsOrNotesToTicks(bars[f.Bar-1], options.FermataRest),
		}
		err := adjustFermata(mid, &tf)
		if err != nil {
			return err
		}
		fermataTick = append(fermataTick, tf)
	}
	var preludeTick []tickRange
	for _, p := range options.Prelude {
		begin, end := p.ToTick(bars)
		begin, err := adjustToNoNotes(mid, begin, options.MaxAdjust)
		if err != nil {
			return err
		}
		end, err = adjustToNoNotes(mid, end, options.MaxAdjust)
		if err != nil {
			return err
		}
		preludeTick = append(preludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], options.RestBetweenVerses)
	totalTicks := bars[len(bars)-1].End()

	log.Printf("fermata data: %+v", fermataTick)

	// Make a whole-file MIDI.
	var preludeCuts []cut
	for _, p := range preludeTick {
		// Prelude does not execute fermatas.
		preludeCuts = append(preludeCuts, cut{
			RestBefore: 0,
			Begin:      p.Begin,
			End:        p.End,
			RestAfter:  0,
		})
	}
	log.Printf("prelude cuts: %+v", preludeCuts)
	verseCuts := fermatize(cut{
		RestBefore: ticksBetweenVerses,
		Begin:      0,
		End:        totalTicks,
	}, fermataTick)
	log.Printf("verse cuts: %+v", preludeCuts)

	if out != "" {
		var cuts []cut
		cuts = append(cuts, preludeCuts...)
		for i := 0; i < options.NumVerses; i++ {
			cuts = append(cuts, verseCuts...)
		}
		wholeMIDI, err := cutMIDI(mid, trim(cuts))
		if err != nil {
			return err
		}
		err = wholeMIDI.WriteFile(out)
		if err != nil {
			return err
		}
		newBars := findBars(wholeMIDI)
		dumpTimeSig("Whole", newBars)
	}

	if outPrefix != "" {
		if len(preludeCuts) > 0 {
			preludeMIDI, err := cutMIDI(mid, trim(preludeCuts))
			if err != nil {
				return err
			}
			err = preludeMIDI.WriteFile(fmt.Sprintf("%s.prelude.mid", outPrefix))
			if err != nil {
				return err
			}
			newBars := findBars(preludeMIDI)
			dumpTimeSig("Prelude", newBars)
		}
		for i, c := range verseCuts {
			sectionMIDI, err := cutMIDI(mid, trim([]cut{c}))
			if err != nil {
				return err
			}
			err = sectionMIDI.WriteFile(fmt.Sprintf("%s.part%d.mid", outPrefix, i))
			if err != nil {
				return err
			}
			newBars := findBars(sectionMIDI)
			dumpTimeSig(fmt.Sprintf("Section %d", i), newBars)
		}
		panicMIDI, err := panicMIDI(mid)
		if err != nil {
			return err
		}
		err = panicMIDI.WriteFile(fmt.Sprintf("%s.panic.mid", outPrefix))
		if err != nil {
			return err
		}
	}

	return nil
}

func trim(c []cut) []cut {
	if len(c) == 0 {
		return c
	}
	result := append([]cut{}, c...)
	result[0].RestBefore = 0
	result[len(result)-1].RestAfter = 0
	return result
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func adjustToNoNotes(mid *smf.SMF, tick, maxDelta int64) (int64, error) {
	// Look for a tick with zero notes playing at start.
	tracker := newNoteTracker(false)
	var bestTick, maxTick int64
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if !tracker.Playing() {
			log.Printf("nothing at %v .. %v", maxTick+1, time)
			if abs(time-tick) < abs(bestTick-tick) {
				bestTick = time
			}
		}
		maxTick = time
		tracker.Handle(track, msg)
		if time > tick+maxDelta {
			return StopIteration
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	// Past EOF?
	if tick >= maxTick && !tracker.Playing() {
		bestTick = tick
	}
	if abs(bestTick-tick) > maxDelta {
		return 0, fmt.Errorf("no noteless tick found around %v (best: %v, max: %v)", tick, bestTick, maxTick)
	}
	log.Printf("adjusted %v -> %v", tick, bestTick)
	return bestTick, nil
}

func adjustFermata(mid *smf.SMF, tf *tickFermata) error {
	fermataNotes := map[Key]struct{}{}
	first := true
	var firstTick int64
	haveHoldTick := false
	waitingForNote := false
	finished := false
	tracker := newNoteTracker(false)
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if time < tf.tick {
			tracker.Handle(track, msg)
			return nil
		}

		// The start tick shall use the UNION of notes played and released.
		if first || time == firstTick {
			first = false
			firstTick = time
			for _, k := range tracker.NotesPlaying() {
				fermataNotes[k] = struct{}{}
			}
		}
		tracker.Handle(track, msg)
		if time == firstTick {
			for _, k := range tracker.NotesPlaying() {
				fermataNotes[k] = struct{}{}
			}
		}

		anyMissing := false
		allMissing := true
		for k := range fermataNotes {
			if tracker.NotePlaying(k) {
				allMissing = false
			} else {
				anyMissing = true
			}
		}
		if anyMissing && !haveHoldTick {
			tf.holdTick = time
			haveHoldTick = true
		}
		if allMissing {
			waitingForNote = true
		}
		if waitingForNote && tracker.Playing() {
			tf.releaseTick = time
			finished = true
			return StopIteration
		}
		return nil
	})
	if err != nil {
		return err
	}
	if first {
		return fmt.Errorf("fermata out of range: %v", tf.tick)
	}
	if !finished {
		tf.releaseTick = -1
	}
	// releaseTick already plays the new notes. Thus, if note end events are in the same tick as releasing, end the notes a bit earlier.
	if tf.holdTick == tf.releaseTick && tf.holdTick > tf.tick {
		tf.holdTick--
	}
	return nil
}

func fermatize(c cut, fermataTick []tickFermata) []cut {
	var result []cut
	for _, tf := range fermataTick {
		if tf.holdTick >= c.Begin && tf.holdTick < c.End && tf.releaseTick < c.End {
			result = append(result,
				cut{
					RestBefore: c.RestBefore,
					Begin:      c.Begin,
					End:        tf.holdTick,
					RestAfter:  tf.extend,
					DirtyBegin: c.DirtyBegin,
					DirtyEnd:   true,
				})
			if tf.releaseTick >= 0 {
				result = append(result,
					cut{
						RestBefore:       0,
						Begin:            tf.holdTick,
						End:              tf.releaseTick,
						RestAfter:        tf.rest,
						DirtyBegin:       true,
						DirtyEnd:         false,
						AllNotesOffAtEnd: true,
					})
				c.Begin = tf.releaseTick
				c.RestBefore = 0
				c.DirtyBegin = false
			} else {
				c.Begin = tf.holdTick
				c.RestBefore = 0
				c.DirtyBegin = true
			}
		}
	}
	return append(result, c)
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
				log.Printf("%s: %d @ %d: %d bar%s of %d/%d", prefix, start+1, startTicks, i-start, plural, sigBar.Num, sigBar.Denom)
			}
			start = i
			if thisBar != nil {
				startTicks = thisBar.Begin
			}
			sigBar = thisBar
		}
	}
	for i, bar := range b {
		gotSig(i, &bar)
	}
	gotSig(len(b), nil)
	log.Printf("%s: %d: end", prefix, len(b)+1)
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

// removeUnneededEvents removes events we do not care about.
func removeUnneededEvents(mid *smf.SMF) error {
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if msg.IsOneOf(midi.ControlChangeMsg, midi.ProgramChangeMsg) {
			return nil
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

// removeRedundantNoteEvents removes overlapping note start events in the song.
func removeRedundantNoteEvents(mid *smf.SMF, refcounting bool) error {
	tracker := newNoteTracker(refcounting)
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		include, track := tracker.Handle(track, msg)
		if !include {
			return nil
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

// forceTempo adjust the tempo.
func forceTempo(mid *smf.SMF, bpm float64) error {
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	tracks[0] = append(tracks[0], smf.Event{
		Delta:   0,
		Message: smf.MetaTempo(bpm),
	})
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if msg.Is(smf.MetaTempoMsg) {
			return nil
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
