package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"gitlab.com/gomidi/midi/v2/smf"
)

type Pos struct {
	Bar       int
	Beat      int
	BeatNum   int
	BeatDenom int
}

func (p Pos) MarshalJSON() ([]byte, error) {
	if p.BeatNum > 0 {
		return json.Marshal(fmt.Sprintf("%d.%d+%d/%d", p.Bar, p.Beat, p.BeatNum, p.BeatDenom))
	}
	return json.Marshal(fmt.Sprintf("%d.%d", p.Bar, p.Beat))
}

var (
	posFlagValue = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?$`)
)

func (p *Pos) UnmarshalJSON(buf []byte) error {
	if string(buf) == "null" {
		return nil
	}
	var item string
	if err := json.Unmarshal(buf, &item); err != nil {
		return err
	}
	result := posFlagValue.FindStringSubmatch(item)
	if result == nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n", item)
	}
	*p = Pos{
		Beat:      1,
		BeatNum:   0,
		BeatDenom: 1,
	}
	var err error
	p.Bar, err = strconv.Atoi(result[1])
	if err != nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
	}
	if result[2] != "" {
		p.Beat, err = strconv.Atoi(result[2])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	if result[3] != "" {
		p.BeatNum, err = strconv.Atoi(result[3])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	if result[4] != "" {
		p.BeatDenom, err = strconv.Atoi(result[4])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", item)
		}
	}
	return nil
}

func (p Pos) ToTick(b bars) int64 {
	return b.ToTick(p.Bar-1, p.Beat-1, p.BeatNum, p.BeatDenom)
}

type Range struct {
	Begin Pos `json:"begin"`
	End   Pos `json:"end"`
}

func (r Range) ToTick(b bars) (int64, int64) {
	return r.Begin.ToTick(b), r.End.ToTick(b)
}

func beatsOrNotesToTicks(b bar, n int) int64 {
	if n < 0 {
		// Negative: this uses denominator ticks.
		return -int64(n) * b.NumLength()
	} else {
		// Positive: this uses beats.
		return int64(n) * b.BeatLength()
	}
}

// Config define global settings.
type Config struct {
	// Organ specific configuration or override.
	BPMFactor              float64 `json:"bpm_factor,omitempty"`
	Channel                int     `json:"channel,omitempty"`
	MelodyTrackNameRE      string  `json:"melody_track_name_re",omitempty`
	MelodyChannel          int     `json:"melody_channel",omitempty`
	BassTrackNameRE        string  `json:"bass_track_name_re",omitempty`
	BassChannel            int     `json:"bass_channel",omitempty`
	HoldRedundantNotes     bool    `json:"hold_redundant_notes,omitempty"`
	FermataExtendBeats     int     `json:"fermata_extend_beats,omitempty"`
	FermataRestBeats       int     `json:"fermata_rest_beats,omitempty"`
	RestBetweenVersesBeats int     `json:"rest_between_verses_beats,omitempty"`

	// Also future options:
	// - Transpose

	PreludePlayerRepeat   int     `json:"prelude_player_repeat,omitempty"`
	PreludePlayerSleepSec float64 `json:"prelude_player_sleep_sec,omitempty"`
}

// Options define file specific options.
type Options struct {
	InputFile      string  `json:"input_file"`
	Fermatas       []Pos   `json:"fermatas,omitempty"`
	Prelude        []Range `json:"prelude,omitempty"`
	NumVerses      int     `json:"num_verses,omitempty"`
	QPMOverride    float64 `json:"qpm_override,omitempty"`
	BPMFactor      float64 `json:"bpm_factor,omitemoty"`
	MaxAdjust      int64   `json:"max_adjust,omitempty"`
	KeepEventOrder bool    `json:"keep_event_order,omitempty"`
	MelodyTracks   []int   `json:"melody_tracks,omitempty"`
	BassTracks     []int   `json:"bass_tracks,omitempty"`
}

func withDefault[T comparable](a, b T) T {
	var empty T
	if a == empty {
		return b
	}
	return a
}

// Process processes the given MIDI file and writes the result to out.
func Process(mid *smf.SMF, config *Config, options *Options) (map[string]*smf.SMF, error) {
	bars := findBars(mid)
	log.Printf("bars: %+v", bars)
	dumpTimeSig("Before", mid, bars)

	// Fix bad events.
	err := removeUnneededEvents(mid)
	if err != nil {
		return nil, err
	}

	// Remove duplicate note start.
	err = removeRedundantNoteEvents(mid, false, config.HoldRedundantNotes)
	if err != nil {
		return nil, err
	}

	// Map all to MIDI channel 2 for the organ.
	err = mapToChannel(mid, config.Channel-1, config.MelodyTrackNameRE, options.MelodyTracks, config.MelodyChannel-1, config.BassTrackNameRE, options.BassTracks, config.BassChannel-1)
	if err != nil {
		return nil, err
	}

	// Fix overlapping notes, as mapToChannel can cause them.
	err = removeRedundantNoteEvents(mid, true, config.HoldRedundantNotes)
	if err != nil {
		return nil, err
	}

	// Sort NoteOff first.
	//
	// This has to take place after channel remapping, as that may remove events.
	if !options.KeepEventOrder {
		err := sortNoteOffFirst(mid)
		if err != nil {
			return nil, err
		}
	}

	if options.QPMOverride > 0 {
		err = forceTempo(mid, options.QPMOverride)
		if err != nil {
			return nil, err
		}
	}

	f := 1.0
	if options.BPMFactor > 0 {
		f *= options.BPMFactor
	}
	if config.BPMFactor > 0 {
		f *= config.BPMFactor
	}
	if f != 1.0 {
		err = adjustTempo(mid, f)
		if err != nil {
			return nil, err
		}
	}

	// Convert all values to ticks.
	var fermataTick []tickFermata
	for _, f := range options.Fermatas {
		tf := tickFermata{
			tick:   f.ToTick(bars),
			extend: beatsOrNotesToTicks(bars[f.Bar-1], withDefault(config.FermataExtendBeats, 1)),
			rest:   beatsOrNotesToTicks(bars[f.Bar-1], withDefault(config.FermataRestBeats, 1)),
		}
		err := adjustFermata(mid, &tf)
		if err != nil {
			return nil, err
		}
		fermataTick = append(fermataTick, tf)
	}
	var preludeTick []tickRange
	for _, p := range options.Prelude {
		begin, end := p.ToTick(bars)
		begin, err := adjustToNoNotes(mid, begin, withDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		end, err = adjustToNoNotes(mid, end, withDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		preludeTick = append(preludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], withDefault(config.RestBetweenVersesBeats, 1))
	totalTicks := bars[len(bars)-1].End()

	log.Printf("fermata data: %+v", fermataTick)

	// Make a whole-file MIDI.
	var preludeCuts []cut
	for _, p := range preludeTick {
		// Prelude does not execute fermatas.
		preludeCuts = append(preludeCuts, cut{
			RestBefore: 0,
			Begin:      p.Begin,
			End:        p.End,
			RestAfter:  0,
		})
	}
	log.Printf("prelude cuts: %+v", preludeCuts)
	verseCuts := fermatize(cut{
		RestBefore: ticksBetweenVerses,
		Begin:      0,
		End:        totalTicks,
	}, fermataTick)
	log.Printf("verse cuts: %+v", verseCuts)

	output := map[string]*smf.SMF{}

	var cuts []cut
	cuts = append(cuts, preludeCuts...)
	for i := 0; i < withDefault(options.NumVerses, 1); i++ {
		cuts = append(cuts, verseCuts...)
	}
	wholeMIDI, err := cutMIDI(mid, trim(cuts))
	if err != nil {
		return nil, err
	}
	output["whole"] = wholeMIDI
	//newBars := findBars(wholeMIDI)
	//dumpTimeSig("Whole", wholeMIDI, newBars)

	if len(preludeCuts) > 0 {
		preludeMIDI, err := cutMIDI(mid, trim(preludeCuts))
		if err != nil {
			return nil, err
		}
		output["prelude"] = preludeMIDI
		newBars := findBars(preludeMIDI)
		dumpTimeSig("Prelude", preludeMIDI, newBars)
	}
	if len(verseCuts) > 0 {
		verseMIDI, err := cutMIDI(mid, trim(verseCuts))
		if err != nil {
			return nil, err
		}
		output["verse"] = verseMIDI
		//newBars := findBars(verseMIDI)
		//dumpTimeSig("Verse", verseMIDI, newBars)
	}
	for i, c := range verseCuts {
		sectionMIDI, err := cutMIDI(mid, trim([]cut{c}))
		if err != nil {
			return nil, err
		}
		output[fmt.Sprintf("part%d", i)] = sectionMIDI
		newBars := findBars(sectionMIDI)
		dumpTimeSig(fmt.Sprintf("Section %d", i), sectionMIDI, newBars)
	}
	panicMIDI, err := panicMIDI(mid)
	if err != nil {
		return nil, err
	}
	output["panic"] = panicMIDI

	return output, nil
}
