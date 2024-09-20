package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

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
