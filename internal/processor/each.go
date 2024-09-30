package processor

import (
	"errors"

	"gitlab.com/gomidi/midi/v2/smf"
)

// StopIteration can be returned to return without failure.
var StopIteration = errors.New("ForEachEventWithTime: StopIteration")

// ForEachEventWithTime runs the given function for each event, with current absolute time and other info.
func ForEachEventWithTime(mid *smf.SMF, yield func(time int64, track int, msg smf.Message) error) error {
	// trackPos is the index of the NEXT event from each track.
	trackPos := make([]int, len(mid.Tracks))
	// trackTime is the time of the LAST event from each track.
	trackTime := make([]int64, len(mid.Tracks))
	for {
		earliestTrack := -1
		var earliestTime int64
		var earliestNoteOff bool
		for i, t := range mid.Tracks {
			p := trackPos[i]
			if p >= len(t) {
				// End of track.
				continue
			}
			time := trackTime[i] + int64(t[p].Delta)
			noteOff := t[p].Message.GetNoteEnd(nil, nil)
			if earliestTrack < 0 || time < earliestTime || (time == earliestTime && noteOff && !earliestNoteOff) {
				earliestTime = time
				earliestTrack = i
				earliestNoteOff = noteOff
			}
		}
		if earliestTrack < 0 {
			// End of MIDI.
			return nil
		}
		msg := mid.Tracks[earliestTrack][trackPos[earliestTrack]].Message
		if !msg.Is(smf.MetaEndOfTrackMsg) {
			err := yield(earliestTime, earliestTrack, mid.Tracks[earliestTrack][trackPos[earliestTrack]].Message)
			if errors.Is(err, StopIteration) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		trackPos[earliestTrack]++
		trackTime[earliestTrack] = earliestTime
	}
}
