package processor

import (
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
)

// panicMIDI generates a new MIDI file that turns all notes off that the input file ever plays.
func panicMIDI(mid *smf.SMF) (*smf.SMF, error) {
	notes := map[Key]struct{}{}
	err := forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		var ch, note uint8
		if msg.GetNoteStart(&ch, &note, nil) {
			k := Key{ch, note}
			notes[k] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var track smf.Track
	for k := range notes {
		track.Add(0, midi.NoteOff(k.ch, k.note))
	}
	track.Close(0)
	newMIDI := smf.New()
	newMIDI.TimeFormat = mid.TimeFormat
	newMIDI.Add(track)
	return newMIDI, nil
}
