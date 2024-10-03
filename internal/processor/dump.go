package processor

import (
	"log"

	"gitlab.com/gomidi/midi/v2/smf"
)

// dumpTimeSig prints the time signatures the song uses in concise form.
func dumpTimeSig(prefix string, mid *smf.SMF, b bars) {
	ForEachEventWithTime(mid, func(time int64, track int, msg smf.Message) error {
		var bpm float64
		if !msg.GetMetaTempo(&bpm) {
			return nil
		}
		bar, beat := b.FromTick(time)
		log.Printf("%s: %d.(%v) @ %d: tempo is %f bpm.", prefix, bar+1, beat+1, time, bpm)
		return nil
	})
	var start int
	var startTicks int64
	var sigBar *bar
	gotSig := func(i int, thisBar *bar) {
		if thisBar == nil || sigBar == nil || thisBar.Num != sigBar.Num || thisBar.Denom != sigBar.Denom {
			if i != start {
				plural := "s"
				if i-start == 1 {
					plural = ""
				}
				log.Printf("%s: %d @ %d: %d bar%s of %d/%d (beat = %d/%d).", prefix, start+1, startTicks, i-start, plural, sigBar.Num, sigBar.Denom, sigBar.BeatNum, sigBar.Denom)
			}
			start = i
			if thisBar != nil {
				startTicks = thisBar.Begin
			}
			sigBar = thisBar
		}
	}
	for i, bar := range b {
		gotSig(i, &bar)
	}
	gotSig(len(b), nil)
	log.Printf("%s: %d: end.", prefix, len(b)+1)
}
