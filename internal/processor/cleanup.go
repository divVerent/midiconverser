package processor

import (
	"log"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

// removeUnneededEvents removes events we do not care about.
func removeUnneededEvents(mid *smf.SMF) error {
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
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

// removeRedundantTempoEvents removes multiple tempi at the same timestamp.
func removeRedundantTempoEvents(mid *smf.SMF) (float64, error) {
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	haveLastTempo := false
	var lastTempoTrack int
	var lastTempoIndex int
	var lastTempoTime int64
	var qpm float64
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if msg.GetMetaTempo(&qpm) {
			if time == lastTempoTime && haveLastTempo {
				log.Printf("Removed redundant tempo event at %v.", time)
				tracks[lastTempoTrack][lastTempoIndex].Message = msg // Keep newest tempo.
				return nil                                           // Discard second event.
			}
			lastTempoTrack = track
			lastTempoIndex = len(tracks[track])
			lastTempoTime = time
			haveLastTempo = true
		}
		tracks[track] = append(tracks[track], smf.Event{
			Delta:   uint32(time - trackTime[track]),
			Message: msg,
		})
		trackTime[track] = time
		return nil
	})
	if err != nil {
		return 0, err
	}
	mid.Tracks = tracks
	return qpm, nil
}

// removeRedundantNoteEvents removes overlapping note start events in the song.
func removeRedundantNoteEvents(mid *smf.SMF, refcounting, holding bool) error {
	tracker := NewNoteTracker(refcounting)
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		include, track := tracker.Handle(time, track, msg)
		if !include {
			var ch, note uint8
			if !holding && msg.GetNoteStart(&ch, &note, nil) {
				key := Key{ch: ch, note: note}
				prevStart := tracker.NoteStart(key)
				duration := time - prevStart
				if duration > 0 {
					//log.Printf("Restarting note with already duration %d.", duration)
					// Restart the note by inserting a note-off and a note-on event.
					noteOff := smf.Message(midi.NoteOff(ch, note))
					tracks[track] = append(tracks[track], smf.Event{
						Delta:   uint32(time - trackTime[track]),
						Message: noteOff,
					})
					trackTime[track] = time
					tracker.SetNoteStart(key, time)
				} else {
					//log.Printf("Not restarting note with already duration %d.", duration)
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
