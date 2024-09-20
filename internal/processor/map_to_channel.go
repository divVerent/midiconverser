package processor

import (
	"gitlab.com/gomidi/midi/v2/smf"
)

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(mid *smf.SMF, ch uint8) {
	for _, t := range mid.Tracks {
		for _, ev := range t {
			var evCh uint8
			if ev.Message.GetChannel(&evCh) {
				ev.Message.Bytes()[0] += ch - evCh
			}
			if ev.Message.GetMetaChannel(&evCh) {
				ev.Message.Bytes()[3] += ch - evCh
			}
		}
	}
}
