package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

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
