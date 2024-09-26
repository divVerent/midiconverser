package processor

import (
	"sort"

	"gitlab.com/gomidi/midi/v2/smf"
)

func sortNoteOffFirst(mid *smf.SMF) error {
	for _, t := range mid.Tracks {
		sortNoteOffFirstTrack(t)
	}
	return nil
}

func sortNoteOffFirstTrack(track smf.Track) {
	// Find groups Delta=<n> 0 ...
	// Sort within group by event type (stable, NoteEnd first).
	// Fix up deltas in group.

	fixup := func(begin, end int) {
		if end <= begin+1 {
			return
		}
		delta := track[begin].Delta
		sort.SliceStable(track[begin:end], func(i, j int) bool {
			iOff := track[begin+i].Message.GetNoteEnd(nil, nil)
			jOff := track[begin+j].Message.GetNoteEnd(nil, nil)
			return iOff && !jOff
		})
		track[begin].Delta = delta
		for i := begin + 1; i < end; i++ {
			track[i].Delta = 0
		}
	}

	begin := 0
	for i, ev := range track {
		if ev.Delta != 0 {
			fixup(begin, i)
			begin = i
		}
	}
	fixup(begin, len(track))
}
