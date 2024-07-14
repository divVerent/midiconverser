package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

type Key struct {
	ch, note uint8
}

type activeNote struct {
	noteOnTrack int
	refs        int
}

type noteTracker struct {
	refcounting bool
	activeNotes map[Key]*activeNote
}

func newNoteTracker(refcounting bool) *noteTracker {
	return &noteTracker{
		refcounting: refcounting,
		activeNotes: map[Key]*activeNote{},
	}
}

func (t noteTracker) Playing() bool {
	return len(t.activeNotes) > 0
}

func (t noteTracker) NotesPlaying() []Key {
	var keys []Key
	for k := range t.activeNotes {
		keys = append(keys, k)
	}
	return keys
}

func (t noteTracker) NotePlaying(k Key) bool {
	_, found := t.activeNotes[k]
	return found
}

func (t noteTracker) NoteTrack(k Key) int {
	return t.activeNotes[k].noteOnTrack
}

func (t noteTracker) Handle(track int, msg smf.Message) (bool, int) {
	var ch, note uint8
	if msg.GetNoteStart(&ch, &note, nil) {
		k := Key{ch, note}
		n := t.activeNotes[k]
		result := n == nil
		if result {
			n = &activeNote{
				refs:        1,
				noteOnTrack: track,
			}
			t.activeNotes[k] = n
		} else if t.refcounting {
			n.refs++
		}
		return result, n.noteOnTrack
	}
	if msg.GetNoteEnd(&ch, &note) {
		k := Key{ch, note}
		n := t.activeNotes[k]
		result := n != nil && n.refs == 1
		if t.refcounting {
			n.refs--
			if n.refs == 0 {
				delete(t.activeNotes, k)
			}
		} else {
			delete(t.activeNotes, k)
		}
		if n == nil {
			return result, track
		}
		return result, n.noteOnTrack
	}
	return true, track
}
