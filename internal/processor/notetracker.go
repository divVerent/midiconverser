package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

type Key struct {
	ch, note uint8
}

type noteTracker struct {
	refcounting bool
	activeNotes map[Key]int
}

func newNoteTracker(refcounting bool) *noteTracker {
	return &noteTracker{
		refcounting: refcounting,
		activeNotes: map[Key]int{},
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

func (t noteTracker) Handle(msg smf.Message) bool {
	var ch, note uint8
	if msg.GetNoteStart(&ch, &note, nil) {
		k := Key{ch, note}
		result := t.activeNotes[k] == 0
		if t.refcounting {
			t.activeNotes[k]++
		} else {
			t.activeNotes[k] = 1
		}
		return result
	}
	if msg.GetNoteEnd(&ch, &note) {
		k := Key{ch, note}
		result := t.activeNotes[k] == 1
		if t.refcounting {
			if t.activeNotes[k] > 0 {
				t.activeNotes[k]--
				if t.activeNotes[k] == 0 {
					delete(t.activeNotes, k)
				}
			}
		} else {
			delete(t.activeNotes, k)
		}
		return result
	}
	return true
}
