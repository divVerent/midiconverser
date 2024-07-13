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
	releaseTick int64 // First tick with a note after the fermata.
}

// Process processes the given MIDI file and writes the result to out.
func Process(in, out, outPrefix string, fermatas []Pos, fermataExtend, fermataRest int, preludeSections []Range, restBetweenVerses int, numVerses int) error {
	mid, err := smf.ReadFile(in)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", in, err)
	}
	bars := findBars(mid)
	log.Printf("bars: %+v", bars)
	dumpTimeSig("Before", bars)

	// Remove duplicate note start.
	removeRedundantNoteEvents(mid, false)

	// Map all to MIDI channel 2 for the organ.
	mapToChannel(mid, 1)

	// Fix overlapping notes.
	removeRedundantNoteEvents(mid, true)

	// Convert all values to ticks.
	var fermataTick []tickFermata
	for _, f := range fermatas {
		tf := tickFermata{
			tick:   f.ToTick(bars),
			extend: beatsOrNotesToTicks(bars[f.Bar], fermataExtend),
			rest:   beatsOrNotesToTicks(bars[f.Bar], fermataRest),
		}
		err := adjustFermata(mid, &tf)
		if err != nil {
			return err
		}
		fermataTick = append(fermataTick, tf)
	}
	var preludeTick []tickRange
	for _, p := range preludeSections {
		begin, end := p.ToTick(bars)
		preludeTick = append(preludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], restBetweenVerses)
	totalTicks := bars[len(bars)-1].End()

	// Make a whole-file MIDI.
	var preludeCuts []cut
	for _, p := range preludeTick {
		preludeCuts = append(preludeCuts, fermatize(cut{
			RestBefore: 0,
			Begin:      p.Begin,
			End:        p.End,
			RestAfter:  0,
		}, fermataTick)...)
	}
	verseCuts := fermatize(cut{
		RestBefore: ticksBetweenVerses,
		Begin:      0,
		End:        totalTicks,
	}, fermataTick)

	if out != "" {
		var cuts []cut
		cuts = append(cuts, preludeCuts...)
		for i := 0; i < numVerses; i++ {
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

func adjustFermata(mid *smf.SMF, tf *tickFermata) error {
	fermataNotes := map[Key]struct{}{}
	first := true
	waitingForNote := false
	finished := false
	tracker := newNoteTracker(false)
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		tracker.Handle(msg)
		if time < tf.tick {
			return nil
		}
		if first {
			first = false
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
		if anyMissing {
			tf.holdTick = time
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
	if !finished {
		return fmt.Errorf("failed to adjust fermata at time %v: first=%v waitingForNote=%v", tf.tick, first, waitingForNote)
	}
	return nil
}

func fermatize(c cut, fermataTick []tickFermata) []cut {
	var result []cut
	for _, tf := range fermataTick {
		if tf.holdTick >= c.Begin && tf.releaseTick < c.End {
			result = append(result,
				cut{
					RestBefore: c.RestBefore,
					Begin:      c.Begin,
					End:        tf.holdTick,
					RestAfter:  tf.extend,
					DirtyBegin: c.DirtyBegin,
					DirtyEnd:   true,
				},
				cut{
					RestBefore: 0,
					Begin:      tf.holdTick,
					End:        tf.releaseTick,
					RestAfter:  tf.rest,
					DirtyBegin: c.DirtyBegin,
					DirtyEnd:   false,
				})
			c.RestBefore = 0
			c.Begin = tf.releaseTick
			c.DirtyBegin = false
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
				log.Printf("%s: %d @ %d: %d bar%s of %d/%d", prefix, start, startTicks, i-start, plural, sigBar.Num, sigBar.Denom)
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

// removeRedundantNoteEvents removes overlapping note start events in the song.
func removeRedundantNoteEvents(mid *smf.SMF, refcounting bool) error {
	tracker := newNoteTracker(refcounting)
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if !tracker.Handle(msg) {
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
