package processor

import (
	"fmt"

	"gitlab.com/gomidi/midi/v2/smf"
)

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func adjustToNoNotes(mid *smf.SMF, tick, maxDelta int64) (int64, error) {
	// Look for a tick with zero notes playing at start.
	tracker := NewNoteTracker(false)
	var bestTick, maxTick int64
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if !tracker.Playing() {
			if time > maxTick {
				//log.Printf("Nothing at %v .. %v.", maxTick+1, time)
				if tick >= maxTick+1 && tick <= time {
					bestTick = tick
				}
				if abs(maxTick+1-tick) < abs(bestTick-tick) {
					bestTick = maxTick + 1
				}
			} else {
				//log.Printf("Nothing at %v.", time)
			}
			if abs(time-tick) < abs(bestTick-tick) {
				bestTick = time
			}
		}
		maxTick = time
		tracker.Handle(time, track, msg)
		if time > tick+maxDelta {
			return StopIteration
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	// Past EOF?
	if tick >= maxTick && !tracker.Playing() {
		bestTick = tick
	}
	if abs(bestTick-tick) > maxDelta {
		return 0, fmt.Errorf("no noteless tick found around %v (best: %v, max: %v)", tick, bestTick, maxTick)
	}
	//log.Printf("Adjusted %v -> %v.", tick, bestTick)
	return bestTick, nil
}

func adjustFermata(mid *smf.SMF, tf *tickFermata) error {
	fermataNotes := map[Key]struct{}{}
	first := true
	var firstTick int64
	haveHoldTick := false
	waitingForNote := false
	finished := false
	tracker := NewNoteTracker(false)
	//log.Printf("Fermata %v.", tf)
	err := ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		if time < tf.tick {
			tracker.Handle(time, track, msg)
			return nil
		}

		// The start tick shall use the UNION of notes played and released.
		if first || time == firstTick {
			first = false
			firstTick = time
			for _, k := range tracker.NotesPlaying() {
				//log.Printf("[%d] Add note %v.", time, k)
				fermataNotes[k] = struct{}{}
			}
		}
		//log.Printf("[%d] Event: %v.", time, msg)
		tracker.Handle(time, track, msg)
		if time == firstTick {
			for _, k := range tracker.NotesPlaying() {
				//log.Printf("[%d] Add note %v.", time, k)
				fermataNotes[k] = struct{}{}
			}
		}

		//log.Printf("[%d] Tracker playing: %v.", time, tracker.Playing())
		anyMissing := false
		allMissing := true
		for k := range fermataNotes {
			if tracker.NotePlaying(k) {
				allMissing = false
			} else {
				anyMissing = true
			}
		}
		//log.Printf("[%d] Any missing: %v, all missing: %v.", time, anyMissing, allMissing)
		if anyMissing && !haveHoldTick {
			//log.Printf("[%d] Hold tick: %d.", time, time-1)
			tf.holdTick = time - 1 // Last complete tick. We can't use time, as it already has some note off events.
			haveHoldTick = true
		}
		if allMissing {
			//log.Printf("[%d] Waiting for note...", time)
			waitingForNote = true
		}
		if waitingForNote && tracker.Playing() {
			//log.Printf("[%d] Release tick received, finished.", time)
			tf.releaseTick = time
			finished = true
			return StopIteration
		}
		return nil
	})
	if err != nil {
		return err
	}
	if first {
		return fmt.Errorf("fermata out of range: %v", tf.tick)
	}
	if !finished {
		tf.releaseTick = -1
	}
	// releaseTick already plays the new notes. Thus, if note end events are in the same tick as releasing, end the notes a bit earlier.
	if tf.holdTick == tf.releaseTick && tf.holdTick > tf.tick {
		tf.holdTick--
	}
	return nil
}
