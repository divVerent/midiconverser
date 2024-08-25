package processor

// TODO: Rewrite all without sequencer, as sequencer seems VERY broken.
// Thus, make our own absolute-time structure, then work in there.
// Can retain track structure by remembering which event was on which track.
// Hardest part is that we need bar/beat number <-> abstime mapping.

import (
	"log"
	"slices"

	"gitlab.com/gomidi/midi/v2/smf"
)

type bar struct {
	// bar position.
	Begin  int64
	Length int64
	// Time signature applying to the bar.
	BeatNum int
	Num     int
	Denom   int
	// Helper values.
	OrigBeatNum int
	OrigDenom   int
}

func gcd(a, b int64) int64 {
	c := a % b
	if c == 0 {
		return b
	}
	return gcd(b, c)
}

func lcm(a, b int64) int64 {
	return a * b / gcd(a, b)
}

func reduce(wantDenom int64, num, denom *int64) {
	// First run the gcd algorithm.
	g := gcd(*num, *denom)
	*num /= g
	*denom /= g
	// Then increase the denominator back to wantDenom level.
	d := lcm(*denom, wantDenom)
	f := d / *denom
	*num *= f
	*denom *= f
}

func (b *bar) SetToLength(length int64) {
	num64 := int64(b.Num) * length
	denom64 := int64(b.Denom) * b.Length
	reduce(int64(b.OrigDenom), &num64, &denom64)
	b.Num, b.Denom = int(num64), int(denom64)
	b.BeatNum = b.OrigBeatNum * b.Denom / b.OrigDenom
	b.Length = length
}

func (b bar) BeatLength() int64 {
	return b.NumLength() * int64(b.BeatNum)
}

func (b bar) NumLength() int64 {
	return b.Length / int64(b.Num)
}

func (b bar) End() int64 {
	return b.Begin + b.Length
}

func (b bar) ToTick(beat, beatNum, beatDenom int) int64 {
	beatLen := b.BeatLength()
	return b.Begin + beatLen*int64(beat) + beatLen*int64(beatNum)/int64(beatDenom)
}

func (b bar) FromTick(tick int64) float64 {
	beatLen := b.BeatLength()
	return float64(tick-b.Begin) / float64(beatLen)
}

type bars []bar

func (b bars) ToTick(bar, beat, beatNum, beatDenom int) int64 {
	if bar == len(b) && beat == 0 && beatNum == 0 {
		return b[len(b)-1].End()
	}
	return b[bar].ToTick(beat, beatNum, beatDenom)
}

func (b bars) FromTick(tick int64) (int, float64) {
	last := len(b) - 1
	for i, bar := range b {
		if i == last || tick < bar.End() {
			return i, bar.FromTick(tick)
		}
	}
	return 0, -1
}

func findBars(midi *smf.SMF) bars {
	type timeSig struct {
		start               int64
		barLen              int64
		beatNum, num, denom int
	}
	sigs := []timeSig{
		{
			start:   0,
			barLen:  4 * int64(midi.TimeFormat.(smf.MetricTicks)),
			beatNum: 1,
			num:     4,
			denom:   4,
		},
	}
	var lastTime int64
	for _, t := range midi.Tracks {
		var time int64
		for _, ev := range t {
			time += int64(ev.Delta)
			// log.Printf("track %d event @ %d: %v", i, time, ev.Message)
			if ev.Message.IsPlayable() && time > lastTime {
				lastTime = time
			}
			var num, denom, cpt, dsqpq uint8
			if ev.Message.GetMetaTimeSig(&num, &denom, &cpt, &dsqpq) {
				whole := 4 * int64(midi.TimeFormat.(smf.MetricTicks))
				beat := int64(midi.TimeFormat.(smf.MetricTicks)) * int64(cpt) / 24
				if (beat*int64(denom))%whole != 0 {
					log.Panicf("unusual beat duration: got %d ticks per beat and %d ticks per whole in a %d/%d time signature",
						beat, whole, num, denom)
				}
				beatNum := beat * int64(denom) / whole
				sigs = append(sigs, timeSig{
					start:   time,
					barLen:  whole * int64(num) / int64(denom),
					beatNum: int(beatNum),
					num:     int(num),
					denom:   int(denom),
				})
			}
		}
	}
	if lastTime == 0 {
		return nil
	}
	sigs = append(sigs, timeSig{
		start: lastTime,
		denom: 0,
	})
	// If there are multiple time signatures at the same start time, only keep the LAST one.
	// As CompactFunc keeps the first of a set of duplicates, we first reverse and then call CompactFunc.
	slices.Reverse(sigs)
	slices.SortStableFunc(sigs, func(a, b timeSig) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return +1
		}
		return 0
	})
	sigs = slices.CompactFunc(sigs, func(a, b timeSig) bool {
		return a.start == b.start
	})
	// Now build the bars structure.
	var time int64
	sigsPos := 0
	var b bars
	for {
		sig := sigs[sigsPos]
		newBar := bar{
			Begin:       time,
			Length:      sig.barLen,
			BeatNum:     sig.beatNum,
			Num:         sig.num,
			Denom:       sig.denom,
			OrigBeatNum: sig.beatNum,
			OrigDenom:   sig.denom,
		}
		nextSig := sigs[sigsPos+1]
		if time+newBar.Length >= nextSig.start {
			// Output a partial bar.
			newBar.SetToLength(nextSig.start - time)
			b = append(b, newBar)
			time = nextSig.start
			sigsPos++
			if sigsPos+1 == len(sigs) {
				break
			}
			continue
		}
		b = append(b, newBar)
		time += newBar.Length
	}
	// Round the last bar up to whole beats.
	lastBar := &b[len(b)-1]
	beatLen := lastBar.BeatLength()
	lastBeats := (lastBar.Length + beatLen - 1) / beatLen
	lastBar.SetToLength(lastBeats * beatLen)
	return b
}
