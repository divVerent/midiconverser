package processor

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"gitlab.com/gomidi/midi/v2/smf"
)

type Pos struct {
	Bar       int
	Beat      int
	BeatNum   int
	BeatDenom int
}

var (
	_ yaml.Marshaler   = Pos{}
	_ yaml.Unmarshaler = &Pos{}
)

func (p Pos) MarshalYAML() (interface{}, error) {
	if p.BeatNum > 0 {
		return fmt.Sprintf("%d.%d+%d/%d", p.Bar, p.Beat, p.BeatNum, p.BeatDenom), nil
	}
	return fmt.Sprintf("%d.%d", p.Bar, p.Beat), nil
}

var (
	posFlagValue = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\+(\d+)/(\d+))?$`)
)

func (p *Pos) UnmarshalYAML(value *yaml.Node) error {
	var item *string
	err := value.Decode(&item)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}
	result := posFlagValue.FindStringSubmatch(*item)
	if result == nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n", *item)
	}
	*p = Pos{
		Beat:      1,
		BeatNum:   0,
		BeatDenom: 1,
	}
	p.Bar, err = strconv.Atoi(result[1])
	if err != nil {
		return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", *item)
	}
	if result[2] != "" {
		p.Beat, err = strconv.Atoi(result[2])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", *item)
		}
	}
	if result[3] != "" {
		p.BeatNum, err = strconv.Atoi(result[3])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", *item)
		}
	}
	if result[4] != "" {
		p.BeatDenom, err = strconv.Atoi(result[4])
		if err != nil {
			return fmt.Errorf("failed to parse --fermatas: pos %q not in format n.n+n/n-n.n+n/n", *item)
		}
	}
	return nil
}

func (p Pos) ToTick(b bars) int64 {
	return b.ToTick(p.Bar-1, p.Beat-1, p.BeatNum, p.BeatDenom)
}

type Range struct {
	Begin Pos `yaml:"begin"`
	End   Pos `yaml:"end"`
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
	// Hymnbook specific configuration. Not needed in UI.
	MelodyTrackNameRE string `yaml:"melody_track_name_re,omitempty"`
	BassTrackNameRE   string `yaml:"bass_track_name_re,omitempty"`

	// Organ specific configuration or override. Should be offered as UI element.
	Channel            int  `yaml:"channel,omitempty"`
	MelodyChannel      int  `yaml:"melody_channel,omitempty"`
	BassChannel        int  `yaml:"bass_channel,omitempty"`
	HoldRedundantNotes bool `yaml:"hold_redundant_notes,omitempty"`

	// Organist preferences. Should be offered as UI element.
	BPMFactor             float64 `yaml:"bpm_factor,omitempty"`
	PreludePlayerRepeat   int     `yaml:"prelude_player_repeat,omitempty"`
	PreludePlayerSleepSec float64 `yaml:"prelude_player_sleep_sec,omitempty"`

	// Interpreted fermatas. Only used for prelude and postlude. Not needed in UI.
	FermatasInPrelude  bool `yaml:"fermatas_in_prelude,omitempty"`
	FermatasInPostlude bool `yaml:"fermatas_in_postlude,omitempty"`
	FermataExtendBeats int  `yaml:"fermata_extend_beats,omitempty"`
	FermataRestBeats   int  `yaml:"fermata_rest_beats,omitempty"`

	// Also future options:
	// - Transpose

	// Misc for exporting. Not needed in UI.
	RestBetweenVersesBeats int     `yaml:"rest_between_verses_beats,omitempty"`
	WholeExportSleepSec    float64 `yaml:"whole_export_sleep_sec,omitempty"`
}

// Options define file specific options.
type Options struct {
	// Managed by the main program right now.
	InputFile       string `yaml:"input_file"`
	InputFileSHA256 string `yaml:"input_file_sha256,omitempty"`

	// For this module.
	Fermatas           []Pos   `yaml:"fermatas,omitempty"`
	Prelude            []Range `yaml:"prelude,omitempty"`
	Postlude           []Range `yaml:"postlude,omitempty"`
	NumVerses          int     `yaml:"num_verses,omitempty"`
	QPMOverride        float64 `yaml:"qpm_override,omitempty"`
	BPMFactor          float64 `yaml:"bpm_factor,omitempty"`
	MaxAdjust          int64   `yaml:"max_adjust,omitempty"`
	KeepEventOrder     bool    `yaml:"keep_event_order,omitempty"`
	MelodyTracks       []int   `yaml:"melody_tracks,omitempty"`
	BassTracks         []int   `yaml:"bass_tracks,omitempty"`
	FermatasInPrelude  *bool   `yaml:"fermatas_in_prelude,omitempty"`
	FermatasInPostlude *bool   `yaml:"fermatas_in_postlude,omitempty"`

	// Tags for automatic selection for prelude.
	Tags []string `yaml:"tags,omitempty"`

	// Pure comment fields. Declared here to preserve them when rewriting the checksum.
	// Can't use YAML # comments because yq loses them.
	Comment string `yaml:"_comment,omitempty"`
}

func WithDefault[T comparable](a, b T) T {
	var empty T
	if a == empty {
		return b
	}
	return a
}

func WithDefaultPtr[T comparable](a *T, b T) T {
	if a == nil {
		return b
	}
	return *a
}

type SpecialPart int

const (
	// Single indicates that this file covers a single part of a verse.
	Single SpecialPart = iota
	// Whole indicates that this file covers the whole output with prelude, all verses and postlude.
	Whole
	// Prelude indicates that this file covers the prelude.
	Prelude
	// Verse indicates that this file covers an entire verse.
	Verse
	// Postlude indicates that this file covers the postlude.
	Postlude
	// Panic indicates that this file just stops all notes.
	Panic
)

type OutputKey struct {
	// Special indicates which part this is.
	Special SpecialPart
	// Part indicates the part index in case Special is Single.
	Part int
}

// String converts OutputKey to a string like in a filename.
func (k OutputKey) String() string {
	switch k.Special {
	case Single:
		return fmt.Sprintf("part%d", k.Part)
	case Whole:
		return "whole"
	case Prelude:
		return "prelude"
	case Verse:
		return "verse"
	case Postlude:
		return "postlude"
	case Panic:
		return "panic"
	default:
		return fmt.Sprintf("unknown%d.%d", k.Special, k.Part)
	}
}

// Process processes the given MIDI file and writes the result to out.
func Process(mid *smf.SMF, config *Config, options *Options) (map[OutputKey]*smf.SMF, error) {
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
			extend: beatsOrNotesToTicks(bars[f.Bar-1], WithDefault(config.FermataExtendBeats, 1)),
			rest:   beatsOrNotesToTicks(bars[f.Bar-1], WithDefault(config.FermataRestBeats, 1)),
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
		begin, err := adjustToNoNotes(mid, begin, WithDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		end, err = adjustToNoNotes(mid, end, WithDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		preludeTick = append(preludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	var postludeTick []tickRange
	for _, p := range options.Postlude {
		begin, end := p.ToTick(bars)
		begin, err := adjustToNoNotes(mid, begin, WithDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		end, err = adjustToNoNotes(mid, end, WithDefault(options.MaxAdjust, 64))
		if err != nil {
			return nil, err
		}
		postludeTick = append(postludeTick, tickRange{
			Begin: begin,
			End:   end,
		})
	}
	ticksBetweenVerses := beatsOrNotesToTicks(bars[len(bars)-1], WithDefault(config.RestBetweenVersesBeats, 1))
	totalTicks := bars[len(bars)-1].End()

	log.Printf("fermata data: %+v", fermataTick)

	// Make a whole-file MIDI.
	var preludeCuts []cut
	for _, p := range preludeTick {
		// Prelude does not execute fermatas.
		preludeCuts = append(preludeCuts, maybeFermatize(cut{
			RestBefore: 0,
			Begin:      p.Begin,
			End:        p.End,
			RestAfter:  0,
		}, fermataTick, WithDefaultPtr(options.FermatasInPrelude, config.FermatasInPrelude))...)
	}
	log.Printf("prelude cuts: %+v", preludeCuts)
	verseCuts := fermatize(cut{
		RestBefore: ticksBetweenVerses,
		Begin:      0,
		End:        totalTicks,
	}, fermataTick)
	log.Printf("verse cuts: %+v", verseCuts)
	var postludeCuts []cut
	for _, p := range postludeTick {
		// Prelude does not execute fermatas.
		postludeCuts = append(postludeCuts, maybeFermatize(cut{
			RestBefore: ticksBetweenVerses,
			Begin:      p.Begin,
			End:        p.End,
			RestAfter:  0,
		}, fermataTick, WithDefaultPtr(options.FermatasInPostlude, config.FermatasInPostlude))...)
	}
	log.Printf("postlude cuts: %+v", postludeCuts)

	output := map[OutputKey]*smf.SMF{}

	var cuts []cut
	cuts = append(cuts, preludeCuts...)
	for i := 0; i < WithDefault(options.NumVerses, 1); i++ {
		cuts = append(cuts, verseCuts...)
	}
	cuts = append(cuts, postludeCuts...)
	wholeMIDI, err := cutMIDI(mid, cuts)
	if err != nil {
		return nil, err
	}
	wholeMIDI, err = trim(wholeMIDI, time.Duration(float64(time.Second)*config.WholeExportSleepSec))
	if err != nil {
		return nil, err
	}
	output[OutputKey{Special: Whole}] = wholeMIDI
	//newBars := findBars(wholeMIDI)
	//dumpTimeSig("Whole", wholeMIDI, newBars)

	if len(preludeCuts) > 0 {
		preludeMIDI, err := cutMIDI(mid, preludeCuts)
		if err != nil {
			return nil, err
		}
		preludeMIDI, err = trim(preludeMIDI, 0)
		if err != nil {
			return nil, err
		}
		output[OutputKey{Special: Prelude}] = preludeMIDI
		newBars := findBars(preludeMIDI)
		dumpTimeSig("Prelude", preludeMIDI, newBars)
	}
	if len(verseCuts) > 0 {
		verseMIDI, err := cutMIDI(mid, verseCuts)
		if err != nil {
			return nil, err
		}
		verseMIDI, err = trim(verseMIDI, 0)
		if err != nil {
			return nil, err
		}
		output[OutputKey{Special: Verse}] = verseMIDI
		//newBars := findBars(verseMIDI)
		//dumpTimeSig("Verse", verseMIDI, newBars)
	}
	for i, c := range verseCuts {
		sectionMIDI, err := cutMIDI(mid, []cut{c})
		if err != nil {
			return nil, err
		}
		sectionMIDI, err = trim(sectionMIDI, 0)
		if err != nil {
			return nil, err
		}
		output[OutputKey{Part: i}] = sectionMIDI
		newBars := findBars(sectionMIDI)
		dumpTimeSig(fmt.Sprintf("Section %d", i), sectionMIDI, newBars)
	}
	if len(postludeCuts) > 0 {
		postludeMIDI, err := cutMIDI(mid, postludeCuts)
		if err != nil {
			return nil, err
		}
		postludeMIDI, err = trim(postludeMIDI, 0)
		if err != nil {
			return nil, err
		}
		output[OutputKey{Special: Postlude}] = postludeMIDI
		newBars := findBars(postludeMIDI)
		dumpTimeSig("Postlude", postludeMIDI, newBars)
	}
	panicMIDI, err := panicMIDI(mid)
	if err != nil {
		return nil, err
	}
	output[OutputKey{Special: Panic}] = panicMIDI

	return output, nil
}
