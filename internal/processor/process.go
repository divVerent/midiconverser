package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

type Pos struct {
	Bar       int
	Beat      int
	BeatNum   int
	BeatDenom int
}

func (p Pos) MarshalJSON() ([]byte, error) {
	if p.BeatNum > 0 {
		return json.Marshal(fmt.Sprintf("%d.%d+%d/%d", p.Bar, p.Beat, p.BeatNum, p.BeatDenom))
	}
	return json.Marshal(fmt.Sprintf("%d.%d", p.Bar, p.Beat))
}

var (
	posFlagValue = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?$`)
)

func (p *Pos) UnmarshalJSON(buf []byte) error {
	if string(buf) == "null" {
		return nil
	}
	var item string
	if err := json.Unmarshal(buf, &item); err != nil {
		return err
	}
	result := posFlagValue.FindStringSubmatch(item)
	if result == nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n", item)
	}
	*p = Pos{
		Beat:      1,
		BeatNum:   0,
		BeatDenom: 1,
	}
	var err error
	p.Bar, err = strconv.Atoi(result[1])
	if err != nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
	}
	if result[2] != "" {
		p.Beat, err = strconv.Atoi(result[2])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	if result[3] != "" {
		p.BeatNum, err = strconv.Atoi(result[3])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	if result[4] != "" {
		p.BeatDenom, err = strconv.Atoi(result[4])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	return nil
}

func (p Pos) ToTick(b bars) int64 {
	return b.ToTick(p.Bar-1, p.Beat-1, p.BeatNum, p.BeatDenom)
}

type Range struct {
	Begin Pos `json:"begin"`
	End   Pos `json:"end"`
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
	InputFile         string  `json:"input_file"`
	Fermatas          []Pos   `json:"fermatas,omitempty"`
	FermataExtend     int     `json:"fermata_extend,omitempty"`
	FermataRest       int     `json:"fermata_rest,omitempty"`
	Prelude           []Range `json:"prelude,omitempty"`
	RestBetweenVerses int     `json:"rest_between_verses,omitempty"`
	NumVerses         int     `json:"num_verses,omitempty"`
	BPMOverride       float64 `json:"bpm_override,omitempty"`
	MaxAdjust         int64   `json:"max_adjust,omitempty"`
	HoldRedundant     bool    `json:"hold_redundant,omitempty"`

	// TODO: Option to sort all NoteOff events first in a tick.
	// Relaxes cutting locations, but MAY break things a bit.
	// Default on.
}

func withDefault[T comparable](a, b T) T {
	var empty T
	if a == empty {
		return b
	}
	return a
}

// Process processes the given MIDI file and writes the result to out.
func Process(out, outPrefix string, options *Options) error {
	mid, err := smf.ReadFile(options.InputFile)
	if err != nil {
		return fmt.Errorf("smf.ReadFile(%q): %w", options.InputFile, err)
	}
	bars := findBars(mid)
	log.Printf("bars: %+v", bars)
	dumpTimeSig("Before", mid, bars)

	// Fix bad events.
	err = removeUnneededEvents(mid)
	if err != nil {
		return err
	}

	// Remove duplicate note start.
	err = removeRedundantNoteEvents(mid, false, options.HoldRedundant)
	if err != nil {
		return err
	}

	// Map all to MIDI channel 2 for the organ.
	mapToChannel(mid, 1)
	if err != nil {
		return err
	}

	// Fix overlapping notes, as mapToChannel can cause them.
	err = removeRedundantNoteEvents(mid, true, options.HoldRedundant)
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
			extend: beatsOrNotesToTicks(bars[f.Bar-1], withDefault(options.FermataExtend, 1)),
			rest:   beatsOrNotesToTicks(bars[f.Bar-1], withDefault(options.FermataRest, 1)),
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
		begin, err := adjustToNoNotes(mid, begin, withDefault(options.MaxAdjust, 64))
		if err != nil {
			return err
		}
		end, err = adjustToNoNotes(mid, end, withDefault(options.MaxAdjust, 64))
		if err != nil {
			return err
		}
		preludeTick = append(preludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], withDefault(options.RestBetweenVerses, 1))
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
		for i := 0; i < withDefault(options.NumVerses, 1); i++ {
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
		dumpTimeSig("Whole", wholeMIDI, newBars)
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
			dumpTimeSig("Prelude", preludeMIDI, newBars)
		}
		if len(verseCuts) > 0 {
			sectionMIDI, err := cutMIDI(mid, trim(verseCuts))
			if err != nil {
				return err
			}
			err = sectionMIDI.WriteFile(fmt.Sprintf("%s.verse.mid", outPrefix))
			if err != nil {
				return err
			}
			newBars := findBars(sectionMIDI)
			dumpTimeSig("Verse", sectionMIDI, newBars)
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
			dumpTimeSig(fmt.Sprintf("Section %d", i), sectionMIDI, newBars)
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
			if time > maxTick {
				log.Printf("nothing at %v .. %v", maxTick+1, time)
				if tick >= maxTick+1 && tick <= time {
					bestTick = tick
				}
				if abs(maxTick+1-tick) < abs(bestTick-tick) {
					bestTick = maxTick + 1
				}
			} else {
				log.Printf("nothing at %v", time)
			}
			if abs(time-tick) < abs(bestTick-tick) {
				bestTick = time
			}
		}
		maxTick = time
		tracker.Handle(time, track, msg)
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
	//log.Printf("fermata %v", tf)
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if time < tf.tick {
			tracker.Handle(time, track, msg)
			return nil
		}

		// The start tick shall use the UNION of notes played and released.
		if first || time == firstTick {
			first = false
			firstTick = time
			for _, k := range tracker.NotesPlaying() {
				//log.Printf("[%d] add note %v", time, k)
				fermataNotes[k] = struct{}{}
			}
		}
		//log.Printf("[%d] event: %v", time, msg)
		tracker.Handle(time, track, msg)
		if time == firstTick {
			for _, k := range tracker.NotesPlaying() {
				//log.Printf("[%d] add note %v", time, k)
				fermataNotes[k] = struct{}{}
			}
		}

		//log.Printf("[%d] trackerPlaying=%v", time, tracker.Playing())
		anyMissing := false
		allMissing := true
		for k := range fermataNotes {
			if tracker.NotePlaying(k) {
				allMissing = false
			} else {
				anyMissing = true
			}
		}
		//log.Printf("[%d] anyMissing=%v allMissing=%v", time, anyMissing, allMissing)
		if anyMissing && !haveHoldTick {
			//log.Printf("[%d] holdTick=%d", time, time-1)
			tf.holdTick = time - 1 // Last complete tick. We can't use time, as it already has some note off events.
			haveHoldTick = true
		}
		if allMissing {
			//log.Printf("[%d] waitingForNote", time)
			waitingForNote = true
		}
		if waitingForNote && tracker.Playing() {
			//log.Printf("[%d] releaseTick, finished", time)
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
func dumpTimeSig(prefix string, mid *smf.SMF, b bars) {
	forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		var bpm float64
		if !msg.GetMetaTempo(&bpm) {
			return nil
		}
		bar, beat := b.FromTick(time)
		log.Printf("%s: %d.(%v) @ %d: tempo is %f bpm", prefix, bar+1, beat+1, time, bpm)
		return nil
	})
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
func removeRedundantNoteEvents(mid *smf.SMF, refcounting, holding bool) error {
	tracker := newNoteTracker(refcounting)
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		include, track := tracker.Handle(time, track, msg)
		if !include {
			var ch, note uint8
			if !holding && msg.GetNoteStart(&ch, &note, nil) {
				key := Key{ch: ch, note: note}
				prevStart := tracker.NoteStart(key)
				duration := time - prevStart
				if duration > 0 {
					log.Printf("restarting note with already duration %d", duration)
					// Restart the note by inserting a note-off and a note-on event.
					noteOff := smf.Message(midi.NoteOff(ch, note))
					tracks[track] = append(tracks[track], smf.Event{
						Delta:   uint32(time - trackTime[track]),
						Message: noteOff,
					})
					trackTime[track] = time
					tracker.SetNoteStart(key, time)
				} else {
					log.Printf("not restarting note with already duration %d", duration)
					return nil
				}
			} else {
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
