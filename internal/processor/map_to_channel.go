package processor

import (
	"log"
	"regexp"

	"gitlab.com/gomidi/midi/v2/smf"
)

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(mid *smf.SMF, ch uint8, melodyRE string, melodyTracks []int, melodyCh uint8, bassRE string, bassTracks []int, bassCh uint8) error {
	melody, err := regexp.Compile(melodyRE)
	if err != nil {
		return err
	}
	bass, err := regexp.Compile(bassRE)
	if err != nil {
		return err
	}
	isMelody := make(map[int]bool, len(melodyTracks))
	for _, i := range melodyTracks {
		isMelody[i] = true
	}
	isBass := make(map[int]bool, len(bassTracks))
	for _, i := range bassTracks {
		isBass[i] = true
	}
	for i, t := range mid.Tracks {
		var name string
		for _, ev := range t {
			if !ev.Message.GetMetaTrackName(&name) {
				continue
			}
			break
		}
		log.Printf("track %d name: %s", i, name)
		if melodyTracks == nil && melodyRE != "" && melody.MatchString(name) {
			isMelody[i] = true
		}
		if bassTracks == nil && bassRE != "" && bass.MatchString(name) {
			isBass[i] = true
		}
	}

	if melodyCh == 255 || melodyCh == ch {
		isMelody = nil
	}
	if bassCh == 255 || bassCh == ch {
		isBass = nil
	}

	log.Printf("melody: %v bass: %v", isMelody, isBass)

	numTracks := len(mid.Tracks)
	melodyTrack := numTracks
	if len(isMelody) > 0 {
		numTracks++
	}
	bassTrack := numTracks
	if len(isBass) > 0 {
		numTracks++
	}

	tracks := make([]smf.Track, numTracks)
	trackTime := make([]int64, numTracks)
	err = forEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		out := func(outTrack int, outCh uint8) {
			newMsg := append(smf.Message(nil), msg...)
			if outCh != 255 {
				var evCh uint8
				if newMsg.GetChannel(&evCh) {
					newMsg[0] += outCh - evCh
				}
				if newMsg.GetMetaChannel(&evCh) {
					newMsg[3] += outCh - evCh
				}
			}
			tracks[outTrack] = append(tracks[outTrack], smf.Event{
				Delta:   uint32(time - trackTime[outTrack]),
				Message: newMsg,
			})
			trackTime[outTrack] = time
		}
		out(track, ch)
		if isMelody[track] {
			out(melodyTrack, melodyCh)
		}
		if isBass[track] {
			out(bassTrack, bassCh)
		}
		return nil
	})
	if err != nil {
		return err
	}
	mid.Tracks = tracks
	return nil
}
