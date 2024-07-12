package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

type key struct {
	ch, note uint8
}

type noteTracker struct {
	refcounting bool
	activeNotes map[key]int
}

func newNoteTracker(refcounting bool) *noteTracker {
	return &noteTracker{
		refcounting: refcounting,
		activeNotes: map[key]int{},
	}
}

func (t noteTracker) Playing() bool {
	return len(t.activeNotes) > 0
}

func (t noteTracker) Handle(msg smf.Message) bool {
	var ch, note uint8
	if msg.GetNoteStart(&ch, &note, nil) {
		k := key{ch, note}
		result := t.activeNotes[k] == 0
		if t.refcounting {
			t.activeNotes[k]++
		} else {
			t.activeNotes[k] = 1
		}
		return result
	}
	if msg.GetNoteEnd(&ch, &note) {
		k := key{ch, note}
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
