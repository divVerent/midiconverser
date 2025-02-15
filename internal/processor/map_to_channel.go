package processor

import (
	"log"
	"regexp"
	"strings"

	"gitlab.com/gomidi/midi/v2/smf"
)

// mapToChannel maps all events of the song to the given MIDI channel.
func mapToChannel(mid *smf.SMF, ch int, melodyRE string, melodyTracks []int, melodyCh int, bassRE string, bassTracks []int, bassCh int, soloRE string, soloTracks []int) error {
	if ch < 0 && melodyCh < 0 && bassCh < 0 {
		// No remapping.
		return nil
	}

	melody, err := regexp.Compile(melodyRE)
	if err != nil {
		return err
	}
	isMelody := make(map[int]bool, len(melodyTracks))
	for _, i := range melodyTracks {
		isMelody[i] = true
	}

	bass, err := regexp.Compile(bassRE)
	if err != nil {
		return err
	}
	isBass := make(map[int]bool, len(bassTracks))
	for _, i := range bassTracks {
		isBass[i] = true
	}

	solo, err := regexp.Compile(soloRE)
	if err != nil {
		return err
	}
	isSolo := make(map[int]bool, len(soloTracks))
	for _, i := range soloTracks {
		isSolo[i] = true
	}

	for i, t := range mid.Tracks {
		var name string
		for _, ev := range t {
			// Take the _last_ event for the track name.
			// But only within the first tick.
			if ev.Delta != 0 {
				break
			}
			var text string
			if ev.Message.GetMetaTrackName(&text) {
				name = text
			}
			if ev.Message.GetMetaText(&text) {
				if rest, found := strings.CutPrefix(text, "track_name="); found {
					name = rest
				}
			}
		}
		log.Printf("Track %d name: %s.", i, name)
		if melodyTracks == nil && melodyRE != "" && melody.MatchString(name) {
			isMelody[i] = true
		}
		if bassTracks == nil && bassRE != "" && bass.MatchString(name) {
			isBass[i] = true
		}
		if soloTracks == nil && soloRE != "" && solo.MatchString(name) {
			isSolo[i] = true
		}
	}

	// Disable melody or bass coupler if no special channel is requested.
	if melodyCh < 0 || melodyCh == ch {
		isMelody = nil
	}
	if bassCh < 0 || bassCh == ch {
		isBass = nil
	}

	log.Printf("Melody coupler tracks: %v; bass coupler tracks: %v; solo tracks: %v", isMelody, isBass, isSolo)

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
	err = ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		channels := map[uint8]bool{}
		wrote := false
		out := func(outTrack int, outCh int) {
			wrote = true
			newMsg := append(smf.Message(nil), msg...)
			// Remap channel if requested.
			var evCh uint8
			if newMsg.GetChannel(&evCh) {
				if outCh >= 0 {
					newMsg[0] += uint8(outCh) - evCh
					evCh = uint8(outCh)
				}
			}
			if newMsg.GetMetaChannel(&evCh) {
				if outCh >= 0 {
					newMsg[3] += uint8(outCh) - evCh
					evCh = uint8(outCh)
				}
			}
			if channels[evCh] {
				// Remove coupler dupes.
				return
			}
			channels[evCh] = true
			tracks[outTrack] = append(tracks[outTrack], smf.Event{
				Delta:   uint32(time - trackTime[outTrack]),
				Message: newMsg,
			})
			trackTime[outTrack] = time
		}
		// Couplers first, even if same channel.
		if isMelody[track] {
			out(melodyTrack, melodyCh)
		}
		if isBass[track] {
			out(bassTrack, bassCh)
		}
		if !wrote || !isSolo[track] {
			out(track, ch)
		}
		return nil
	})
	if err != nil {
		return err
	}
	mid.Tracks = tracks
	return nil
}
