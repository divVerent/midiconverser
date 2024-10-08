package processor

import (
	"fmt"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

type tickRange struct {
	Begin, End int64
}

type cut struct {
	RestBefore           int64
	Begin, End           int64
	RestAfter            int64
	DirtyBegin, DirtyEnd bool
	AllNotesOffAtEnd     bool
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
	tracker := NewNoteTracker(false)
	forEachInSection := func(from, to int64, dirtyFrom, dirtyTo bool, yield func(time int64, track int, msg smf.Message) error) error {
		tracker = NewNoteTracker(false) // TODO: make it fully local.
		first := true
		wasPlayingAtEnd := false
		err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
			wasPlaying := tracker.Playing()
			if time > to {
				return StopIteration
			}
			tracker.Handle(time, track, msg)
			if time < from {
				return nil
			}
			isNoteOn := msg.GetNoteStart(nil, nil, nil)
			isNoteOff := msg.GetNoteEnd(nil, nil)
			ignore := (time == from && isNoteOff) || (time == to && !isNoteOff)
			if ignore {
				return nil
			}
			// TODO: Can improve algorithm of this to collect data first then act at end of every tick.
			// Then we can even handle ticks with alternating note off and note on events.
			if first && isNoteOn {
				if !dirtyFrom && wasPlaying {
					return fmt.Errorf("already playing a note at start of section %d..%d to be copied at time %d track %d", from, to, time, track)
				}
				first = false
			}
			wasPlayingAtEnd = tracker.Playing()
			return yield(time, track, msg)
		})
		if err != nil {
			return err
		}
		if !dirtyTo && wasPlayingAtEnd {
			return fmt.Errorf("still playing a note at end of section to be copied at time %d", to)
		}
		return nil
	}
	copyMeta := func(from, to int64, dirtyFrom, dirtyTo bool, outTick int64) error {
		return forEachInSection(from, to, dirtyFrom, dirtyTo, func(time int64, track int, msg smf.Message) error {
			if msg.IsOneOf(midi.NoteOnMsg, midi.NoteOffMsg, midi.ControlChangeMsg, midi.PitchBendMsg, midi.AfterTouchMsg, midi.PolyAfterTouchMsg, midi.ProgramChangeMsg) {
				return nil
			}
			addEvent(track, outTick, msg)
			return nil
		})
	}
	copyAll := func(from, to int64, dirtyFrom, dirtyTo bool, outTick int64) error {
		return forEachInSection(from, to, dirtyFrom, dirtyTo, func(time int64, track int, msg smf.Message) error {
			addEvent(track, outTick+time-from, msg)
			return nil
		})
	}

	prevEndTick := int64(0)
	outTick := int64(0)
	for _, cut := range cuts {
		// For each cut, all non-note events from the previous range's end to the next range's start are repeated.
		if prevEndTick > cut.Begin {
			// If seeking backwards, we have to repeat events from the start.
			prevEndTick = 0
		}
		err := copyMeta(prevEndTick, cut.Begin, true, true, outTick)
		if err != nil {
			return nil, err
		}
		outTick += cut.RestBefore
		err = copyAll(cut.Begin, cut.End, cut.DirtyBegin, cut.DirtyEnd, outTick)
		if err != nil {
			return nil, err
		}
		outTick += cut.End - cut.Begin
		if cut.AllNotesOffAtEnd {
			for _, k := range tracker.NotesPlaying() {
				msg := smf.Message(midi.NoteOff(k.ch, k.note))
				track := tracker.NoteTrack(k)
				addEvent(track, outTick, msg)
				tracker.Handle(cut.End, track, msg)
			}
		}
		outTick += cut.RestAfter
		prevEndTick = cut.End
	}
	for i := range tracks {
		closeTrack(i, outTick)
	}

	newMIDI := smf.NewSMF1()
	newMIDI.TimeFormat = mid.TimeFormat
	for _, t := range tracks {
		newMIDI.Add(t)
	}
	return newMIDI, nil
}
