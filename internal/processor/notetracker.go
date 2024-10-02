package processor

import (
	"sort"

	"gitlab.com/gomidi/midi/v2/smf"
)

type Key struct {
	ch, note uint8
}

func KeySorter(s []Key) func(i, j int) bool {
	return func(i, j int) bool {
		a, b := s[i].ch, s[j].ch
		if a != b {
			return a < b
		}
		a, b = s[i].note, s[j].note
		return a < b
	}
}

type activeNote struct {
	noteOnTrack int
	refs        int
	start       int64
}

type NoteTracker struct {
	refcounting bool
	activeNotes map[Key]*activeNote
}

func NewNoteTracker(refcounting bool) *NoteTracker {
	return &NoteTracker{
		refcounting: refcounting,
		activeNotes: map[Key]*activeNote{},
	}
}

func (t NoteTracker) Playing() bool {
	return len(t.activeNotes) > 0
}

func (t NoteTracker) NotesPlaying() []Key {
	var keys []Key
	for k := range t.activeNotes {
		keys = append(keys, k)
	}
	sort.Slice(keys, KeySorter(keys))
	return keys
}

func (t NoteTracker) NotePlaying(k Key) bool {
	_, found := t.activeNotes[k]
	return found
}

func (t NoteTracker) NoteStart(k Key) int64 {
	return t.activeNotes[k].start
}

func (t NoteTracker) SetNoteStart(k Key, time int64) {
	t.activeNotes[k].start = time
}

func (t NoteTracker) NoteTrack(k Key) int {
	return t.activeNotes[k].noteOnTrack
}

func (t NoteTracker) Handle(time int64, track int, msg smf.Message) (bool, int) {
	var ch, note uint8
	if msg.GetNoteStart(&ch, &note, nil) {
		k := Key{ch, note}
		n := t.activeNotes[k]
		result := n == nil
		if result {
			n = &activeNote{
				refs:        1,
				noteOnTrack: track,
				start:       time,
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
