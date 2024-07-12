package processor

import (
	"fmt"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

type tickRange struct {
	begin, end int64
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
	forEachInSection := func(from, to int64, yield func(time int64, track int, msg smf.Message) error) error {
		first := true
		wasPlayingAtEnd := false
		tracker := newNoteTracker(false)
		err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
			wasPlaying := tracker.Playing()
			tracker.Handle(msg)
			if time < from {
				return nil
			}
			if time >= to {
				return nil
			}
			if first {
				if wasPlaying {
					return fmt.Errorf("already playing a note at start of section to be copied at time %d track %d", time, track)
				}
				first = false
			}
			wasPlayingAtEnd = tracker.Playing()
			return yield(time, track, msg)
		})
		if err != nil {
			return err
		}
		if wasPlayingAtEnd {
			return fmt.Errorf("still playing a note at end of section to be copied at time %d", to)
		}
		return nil
	}
	copyMeta := func(from, to int64, outTick int64) error {
		return forEachInSection(from, to, func(time int64, track int, msg smf.Message) error {
			if msg.IsOneOf(midi.NoteOnMsg, midi.NoteOffMsg, midi.PitchBendMsg, midi.AfterTouchMsg, midi.PolyAfterTouchMsg) {
				return nil
			}
			addEvent(track, outTick, msg)
			return nil
		})
	}
	copyAll := func(from, to int64, outTick int64) error {
		return forEachInSection(from, to, func(time int64, track int, msg smf.Message) error {
			addEvent(track, outTick, msg)
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
	return newMIDI, nil
}
