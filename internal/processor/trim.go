package processor

import (
	"log"
	"math"
	"time"

	"gitlab.com/gomidi/midi/v2/smf"
)

func trim(mid *smf.SMF, extraTimeAtEnd time.Duration) (*smf.SMF, error) {
	// Decide start and length.
	var firstTime int64 = math.MaxInt64
	var lastTime int64 = math.MinInt64
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if !msg.GetNoteStart(nil, nil, nil) && !msg.GetNoteEnd(nil, nil) {
			return nil
		}
		if time < firstTime {
			firstTime = time
		}
		if time > lastTime {
			lastTime = time
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if firstTime > lastTime {
		log.Printf("Writing output file with no events.")
		firstTime = 0
		lastTime = 0
	}

	// Fixup timestamps.
	tracks := make([]smf.Track, len(mid.Tracks))
	trackTime := make([]int64, len(mid.Tracks))
	err = ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if time < firstTime {
			time = 0
		} else if time > lastTime {
			time = lastTime - firstTime
		} else {
			time = time - firstTime
		}
		tracks[track] = append(tracks[track], smf.Event{
			Delta:   uint32(time - trackTime[track]),
			Message: msg,
		})
		trackTime[track] = time
		return nil
	})
	if err != nil {
		return nil, err
	}

	mid.Tracks = tracks
	lastTempo, err := removeRedundantTempoEvents(mid)
	if err != nil {
		return nil, err
	}

	extraTicks := mid.TimeFormat.(smf.MetricTicks).Ticks(lastTempo, extraTimeAtEnd)
	for track := range mid.Tracks {
		mid.Tracks[track].Close(uint32((lastTime-firstTime)-trackTime[track]) + extraTicks)
	}

	newMIDI := smf.NewSMF1()
	newMIDI.TimeFormat = mid.TimeFormat
	for _, t := range mid.Tracks {
		newMIDI.Add(t)
	}

	return newMIDI, nil
}
