package player

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"reflect"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/smf"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

func tagsDefault() map[string]bool {
	tags := map[string]bool{
		"external": false,
		"men":      false,
		"national": false,
		"women":    false,
	}
	// Find out if we're in advent or Christmas.
	now := time.Now()
	endTime := time.Date(now.Year(), 12, 27, 0, 0, 0, 0, time.Local)
	// Latest possible 4th Advent.
	beginTime := time.Date(now.Year(), 12, 24, 0, 0, 0, 0, time.Local)
	// Find real 4th Advent.
	for beginTime.Weekday() != time.Sunday {
		beginTime = beginTime.Add(-24 * time.Hour)
	}
	// Go back to 1st Advent.
	beginTime = beginTime.Add(-3 * 7 * 24 * time.Hour)
	log.Printf("First Advent: %v.", beginTime)
	log.Printf("Xmas ends: %v.", endTime)
	if now.Before(beginTime) || !now.Before(endTime) {
		tags["xmas"] = false
	}
	return tags
}

type Command struct {
	// Exit exits all current playbacks and returns to waiting state.
	Exit bool

	// Quit quits the entire main loop.
	Quit bool

	// PlayOne plays the given file.
	PlayOne string

	// PlayPrelude enters prelude player mode.
	PlayPrelude bool

	// New list of tags for prelude selection.
	PreludeTags map[string]bool

	// Tempo sets the tempo to a new factor.
	Tempo float64

	// NumVerses is an override for the verse count.
	NumVerses int

	// Answer continues the current playback (exits a Prompt state).
	Answer bool

	// Config contains a new configuration. Will be applied on next hymn.
	Config *processor.Config

	// OutPort contains a new, not yet opened, MIDI port. Will be applied once silent.
	OutPort drivers.Out
}

// IsZero returns if the command is an empty message. If so, this likely indicates a closed channel.
func (c Command) IsZero() bool {
	return reflect.DeepEqual(c, Command{})
}

// IsMainLoopCommands returns if the command can only be handled by the main loop.
func (c Command) IsMainLoopCommand() bool {
	return c.Exit || c.Quit || c.PlayOne != "" || c.PlayPrelude || c.IsZero()
}

// UIState is the state of the user interface.
type UIState struct {
	// Err is set to show an error message. The backend is dead, but can be
	// restarted by sending a PlayOne or PlayPrelude message.
	Err error

	// These correspond to the Commands.

	// PlayOne is the name of the currently playing file.
	PlayOne string

	// PlayPrelude indicates if we're in the prelude player.
	PlayPrelude bool

	// List of tags for prelude selection.
	PreludeTags map[string]bool

	// Tempo is the current tempo as a factor of normal.
	Tempo float64

	// Number of verses to play.
	NumVerses int

	// HavePostlude tells if a postlude is pending.
	HavePostlude bool

	// Prompt is the text to prompt the user with.
	// To clear a prompt, send the Answer message.
	Prompt string

	// Informational outputs.

	// CurrentFile is the currently being played file.
	CurrentFile string

	// CurrentPart is the currently being played part.
	CurrentPart processor.OutputKey

	// CurrentMessage is a message for what is currently playing.
	CurrentMessage string

	// Playing is whether we are currently playing.
	Playing bool

	// PlaybackPosTime is the wall time PlaybackPos was last updated.
	PlaybackPosTime time.Time

	// PlaybackPos is the current playback position.
	PlaybackPos time.Duration

	// PlaybackLen is the length of the current file.
	PlaybackLen time.Duration

	// Verse is the current verse.
	Verse int

	// Comment is the hymn comment string.
	Comment string

	// UnrolledNumVerses of real verses in hymn. Only useful if NumVerses == 1.
	UnrolledNumVerses int
}

func (ui UIState) ActualPlaybackPos() time.Duration {
	delta := time.Duration(float64(time.Since(ui.PlaybackPosTime)) * ui.Tempo)
	return ui.PlaybackPos + delta
}

func (ui UIState) ActualPlaybackFraction() float64 {
	return float64(ui.ActualPlaybackPos()) / float64(ui.PlaybackLen)
}

type Backend struct {
	// Commands can be used to send commands to the UI.
	Commands chan Command

	// UIStates receives updates to the UI state non-blockingly.
	UIStates chan UIState

	// fs is the file system.
	fsys fs.FS

	// The configuration data.
	config processor.Config

	// outPort is the MIDI port to play to.
	outPort drivers.Out

	// nextOutPort is the outPort to change to. Will be applied on next
	// playback or when no note is playing.
	nextOutPort drivers.Out

	// The current UI state. Sent to the client on every update, nonblockingly.
	uiState UIState

	// The next command to be executed. Commands that cannot be processed
	// immediately are enqueued here and handled by the main loop. There can be
	// only one.
	nextCommand *Command

	// If set, running the main loop will just play this.
	playOnly string

	// Own copy of prelude tags.
	preludeTags map[string]bool
}

type Options struct {
	// FSys is the virtual file system to use.
	FSys fs.FS

	// Config is the global configuration to use.
	Config *processor.Config

	// OutPort is the MIDI output port to use. OK to change later.
	OutPort drivers.Out

	// PlayOnly is the single file to play.
	PlayOnly string
}

func NewBackend(options *Options) *Backend {
	preludeTags := tagsDefault()
	return &Backend{
		Commands:    make(chan Command, 10),
		UIStates:    make(chan UIState, 100),
		fsys:        options.FSys,
		config:      *options.Config,
		nextOutPort: options.OutPort,
		uiState: UIState{
			PreludeTags:    CopyPreludeTags(preludeTags),
			Tempo:          1.0,
			CurrentMessage: "initializing player",
		},
		nextCommand: nil,
		playOnly:    options.PlayOnly,
		preludeTags: preludeTags,
	}
}

func (b *Backend) sendUIState() {
	select {
	case b.UIStates <- b.uiState:
		return
	default:
		log.Printf("Tried to send an UI state, but nobody came.")
		return
	}
}

var SigIntError = errors.New("SIGINT caught")
var sigInt = make(chan os.Signal, 1)

func init() {
	signal.Notify(sigInt, os.Interrupt)
}

func (b *Backend) sigSleep(t time.Duration) error {
	done := time.After(t)
	for {
		select {
		case <-sigInt:
			return SigIntError
		case cmd := <-b.Commands:
			if err := b.handleCommandDuringSleep(cmd); err != nil {
				if !errors.Is(err, promptAnsweredError) {
					if b.nextCommand != nil {
						log.Panicf("Unreachable code: already have a next command!")
					}
					b.nextCommand = &cmd
				}
				return err
			}
			// Otherwise, the command has been handled, and the loop will run again.
		case <-done:
			return nil
		}
	}
}

var promptAnsweredError = errors.New("prompt answered")
var exitPlaybackError = errors.New("exiting playback")

func CopyPreludeTags(from map[string]bool) map[string]bool {
	to := make(map[string]bool, len(from))
	for k, v := range from {
		to[k] = v
	}
	return to
}

func (b *Backend) handleCommandDuringSleep(cmd Command) error {
	if cmd.IsMainLoopCommand() {
		return exitPlaybackError
	}
	switch {
	case cmd.PreludeTags != nil:
		b.preludeTags = CopyPreludeTags(cmd.PreludeTags)
		b.uiState.PreludeTags = CopyPreludeTags(b.preludeTags)
		b.sendUIState()
		return nil
	case cmd.Tempo != 0:
		b.uiState.Tempo = cmd.Tempo
		b.sendUIState()
		return nil
	case cmd.NumVerses != 0:
		b.uiState.NumVerses = cmd.NumVerses
		b.sendUIState()
		return nil
	case cmd.Answer:
		if b.uiState.Prompt == "" {
			log.Printf("Spurious prompt answer: %+v.", cmd)
			return nil
		}
		return promptAnsweredError // Caught when waiting for prompt.
	case cmd.Config != nil:
		b.config = *cmd.Config
		return nil
	case cmd.OutPort != nil:
		b.nextOutPort = cmd.OutPort
		return nil
	default:
		return fmt.Errorf("unrecognized command: %+v", cmd)
	}
}

func (b *Backend) updateOutPort() error {
	if b.nextOutPort == nil {
		return nil
	}
	port := b.nextOutPort
	b.nextOutPort = nil
	err := port.Open()
	if err != nil {
		return err
	}
	if b.outPort != nil {
		b.outPort.Close()
	}
	b.outPort = port
	return nil
}

// playMIDI plays a MIDI file on the current thread.
func (b *Backend) playMIDI(mid *smf.SMF, key processor.OutputKey) error {
	err := b.updateOutPort()
	if err != nil {
		return err
	}

	var maxTick int64
	err = processor.ForEachEventWithTime(mid, func(t int64, track int, msg smf.Message) error {
		maxTick = t
		return nil
	})
	maxT := time.Microsecond * time.Duration(mid.TimeAt(maxTick))
	b.uiState.Playing = true
	b.uiState.CurrentPart = key
	b.uiState.PlaybackLen = maxT
	b.uiState.PlaybackPos = 0
	b.uiState.PlaybackPosTime = time.Time{}
	b.sendUIState()

	defer func() {
		b.uiState.Playing = false
		b.uiState.CurrentPart = processor.OutputKey{}
		b.uiState.PlaybackLen = 0
		b.uiState.PlaybackPos = 0
		b.uiState.PlaybackPosTime = time.Time{}
		b.sendUIState()
	}()

	var prevTick int64
	var prevT time.Duration
	prevNow := time.Now()

	var fixOffsetT time.Duration

	tracker := processor.NewNoteTracker(false)
	err = processor.ForEachEventWithTime(mid, func(t int64, track int, msg smf.Message) error {
		if msg.IsMeta() {
			return nil
		}

		// Allow port changes if no note is playing right now.
		if !tracker.Playing() {
			err := b.updateOutPort()
			if err != nil {
				return err
			}
		}

		tracker.Handle(t, track, msg)

		// Parse MIDI.
		midiT := time.Microsecond*time.Duration(mid.TimeAt(t)) + fixOffsetT
		midiMsg := midi.Message(msg)

		if midiT < prevT {
			log.Printf("Playback time went backwards from %v to %v for timestamps %v to %v.", prevT, midiT, prevTick, t)
			fixOffsetT += prevT - midiT
			midiT = prevT
		}

		// Back to delta.
		deltaT := midiT - prevT

		prevTick = t
		prevT = midiT

		// Timing.
		newNow := prevNow.Add(time.Duration(float64(deltaT) / b.uiState.Tempo))
		waitTime := newNow.Sub(time.Now())
		if waitTime > 0 {
			err := b.sigSleep(waitTime)
			if err != nil {
				return err
			}
		}
		prevNow = newNow

		b.uiState.PlaybackPos = midiT
		b.uiState.PlaybackPosTime = newNow
		b.sendUIState()

		// Write to output.
		if b.outPort == nil {
			return fmt.Errorf("no output port")
		}
		return b.outPort.Send(midiMsg)
	})

	return err
}

func fixOutput(output map[processor.OutputKey]*smf.SMF) error {
	// Reprocess entire file just in case.
	// This fixes missing tempo change events.
	for k, v := range output {
		var b bytes.Buffer
		_, err := v.WriteTo(&b)
		if err != nil {
			return fmt.Errorf("cannot rewrite MIDI %v: %w", k, err)
		}
		fixed, err := smf.ReadFrom(&b)
		if err != nil {
			return fmt.Errorf("cannot reread MIDI %v: %w", k, err)
		}
		output[k] = fixed
	}
	return nil
}

// process processes the given input.
func (b *Backend) process(options *processor.Options) (map[processor.OutputKey]*smf.SMF, error) {
	output, err := file.Process(b.fsys, &b.config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to process: %w", err)
	}

	err = fixOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to autofix: %w", err)
	}

	return output, nil
}

// preludePlayerOne plays the given file's whole verse for prelude purposes.
func (b *Backend) preludePlayerOne(optionsFile string) (bool, error) {
	options, err := file.ReadOptions(b.fsys, optionsFile)
	if err != nil {
		log.Printf("Skipping prelude file %v due to read error: %v.", optionsFile, err)
		return false, nil
	}

	tags := make(map[string]bool, len(options.Tags))
	for _, t := range options.Tags {
		tags[t] = true
	}

	needWant := false
	haveWant := false
	for k, v := range b.preludeTags {
		if v {
			needWant = true
			if tags[k] {
				haveWant = true
			}
		} else {
			if tags[k] {
				log.Printf("Skipping %s due to no forbidden tags (want no %v).", optionsFile, k)
				return false, nil
			}
		}
	}
	if needWant && !haveWant {
		log.Printf("Skipping %s due to no matching tags (want one of %v).", optionsFile, b.preludeTags)
		return false, nil
	}

	if options.UnrolledNumVerses != 0 {
		log.Printf("Skipping %s due to baked-in repeats.", optionsFile)
		return false, nil
	}

	output, err := b.process(options)
	if err != nil {
		log.Printf("Skipping prelude file %v due to process error: %v.", optionsFile, err)
		return false, nil
	}

	key := processor.OutputKey{Special: processor.Panic}
	allOff := output[key]
	defer b.playMIDI(allOff, key)

	b.uiState.CurrentFile = optionsFile
	// b.sendUIState() // Redundant with playMIDI.
	defer func() {
		b.uiState.CurrentFile = ""
		b.sendUIState()
	}()

	key = processor.OutputKey{Special: processor.Verse}
	verse := output[processor.OutputKey{Special: processor.Verse}]
	if verse == nil {
		return false, fmt.Errorf("no verse file for %v", optionsFile)
	}

	log.Printf("Playing full verses for prelude: %v.", optionsFile)

	b.uiState.NumVerses = processor.WithDefault(b.config.PreludePlayerRepeat, 2) // Cleared by preludePlayer().
	b.uiState.HavePostlude = false
	for i := 0; i < b.uiState.NumVerses; i++ {
		b.uiState.Verse = i // Cleared by preludePlayer().
		err := b.playMIDI(verse, key)
		if err != nil {
			return false, fmt.Errorf("could not play %v: %w", optionsFile, err)
		}
		err = b.sigSleep(time.Duration(float64(time.Second) * processor.WithDefault(b.config.PreludePlayerSleepSec, 2.0)))
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// preludePlayer plays random hymns.
func (b *Backend) preludePlayer() error {
	b.uiState.PlayPrelude = true
	b.uiState.CurrentMessage = "prelude player"
	// b.sendUIState() // Redundant with playMIDI.
	defer func() {
		b.uiState.PlayPrelude = false
		b.uiState.CurrentMessage = ""
		b.uiState.Verse = 0
		b.uiState.NumVerses = 0
		b.uiState.HavePostlude = false
		b.sendUIState()
	}()

	for {
		all, err := fs.Glob(b.fsys, "*.yml")
		if err != nil {
			return fmt.Errorf("glob: %w", err)
		}
		rand.Shuffle(len(all), func(i, j int) {
			all[i], all[j] = all[j], all[i]
		})
		gotOne := false
		for _, f := range all {
			played, err := b.preludePlayerOne(f)
			if err != nil {
				return err
			}
			if played {
				gotOne = true
			}
		}
		if !gotOne {
			return fmt.Errorf("no single prelude file found")
		}
	}
}

// prompt asks the user something.
func (b *Backend) prompt(ask, response string) error {
	b.uiState.Prompt = ask
	b.sendUIState()
	defer func() {
		b.uiState.Prompt = ""
		b.sendUIState()
	}()
	errC := make(chan error, 1)
	go func() {
		for {
			err := b.sigSleep(time.Second)
			if err != nil {
				errC <- err
				return
			}
		}
	}()
	err := <-errC
	if !errors.Is(err, promptAnsweredError) {
		return err
	}
	b.uiState.CurrentMessage = response
	return nil
}

// singlePlayer plays the given file interactively.
func (b *Backend) singlePlayer(optionsFile string) error {
	options, err := file.ReadOptions(b.fsys, optionsFile)
	if err != nil {
		return fmt.Errorf("failed to read %v: %w", optionsFile, err)
	}
	output, err := b.process(options)
	if err != nil {
		return fmt.Errorf("failed to process %v: %w", optionsFile, err)
	}

	key := processor.OutputKey{Special: processor.Panic}
	allOff := output[key]
	defer b.playMIDI(allOff, key)

	log.Printf("Playing all verses of %v.", optionsFile)

	b.uiState.PlayOne = optionsFile
	b.uiState.CurrentFile = optionsFile
	b.uiState.NumVerses = processor.WithDefault(options.NumVerses, 1)
	b.uiState.UnrolledNumVerses = options.UnrolledNumVerses
	b.uiState.Comment = options.Comment
	b.uiState.HavePostlude = output[processor.OutputKey{Special: processor.Postlude}] != nil
	b.uiState.Verse = 0
	// b.sendUIState() // Redundant with prompt.
	defer func() {
		b.uiState.PlayOne = ""
		b.uiState.CurrentFile = ""
		b.uiState.NumVerses = 0
		b.uiState.UnrolledNumVerses = 0
		b.uiState.Comment = ""
		b.uiState.HavePostlude = false
		b.uiState.Verse = 0
		b.uiState.CurrentMessage = "" // Written to by prompt.
		b.sendUIState()
	}()

	key = processor.OutputKey{Special: processor.Prelude}
	prelude := output[key]
	if prelude != nil {
		err := b.prompt("Start Prelude", "playing prelude")
		if err != nil {
			return err
		}
		err = b.playMIDI(prelude, key)
		if err != nil {
			return fmt.Errorf("could not play %v prelude: %w", optionsFile, err)
		}
	}

	for i := 0; i < b.uiState.NumVerses; i++ {
		b.uiState.Verse = i
		n := 0
		for j := 0; ; j++ {
			key := processor.OutputKey{Part: j}
			part := output[key]
			if part == nil {
				break
			}
			n++
		}
		for j := 0; j < n; j++ {
			key := processor.OutputKey{Part: j}
			part := output[key]
			if part == nil {
				break
			}
			var msg string
			if j == 0 {
				msg = "Start Verse"
			} else if j%2 == 1 {
				msg = "End Fermata"
			} else {
				msg = "Continue"
			}
			err := b.prompt(msg, fmt.Sprintf("playing part %d/%d", j+1, n))
			if err != nil {
				return err
			}
			err = b.playMIDI(part, key)
			if err != nil {
				return fmt.Errorf("could not play %v part %v: %w", optionsFile, j, err)
			}
		}
	}

	key = processor.OutputKey{Special: processor.Postlude}
	postlude := output[key]
	if postlude != nil {
		err := b.prompt("Start Postlude", "playing postlude")
		if err != nil {
			return err
		}
		err = b.playMIDI(postlude, key)
		if err != nil {
			return fmt.Errorf("could not play %v postlude: %w", optionsFile, err)
		}
	}

	return nil
}

var QuitError = errors.New("intentionally quitting")

func (b *Backend) handleMainLoopCommand(cmd Command) error {
	if !cmd.IsMainLoopCommand() {
		return b.handleCommandDuringSleep(cmd)
	}
	switch {
	case cmd.Exit:
		return nil
	case cmd.Quit:
		return QuitError
	case cmd.PlayOne != "":
		return b.singlePlayer(cmd.PlayOne)
	case cmd.PlayPrelude:
		return b.preludePlayer()
	case cmd.IsZero():
		return nil
	default:
		return fmt.Errorf("unrecognized main loop command: %+v", cmd)
	}
}

func (b *Backend) Loop() error {
	b.uiState.Err = nil
	b.uiState.CurrentMessage = ""

	// If only one file should be played, set it here.
	if b.playOnly != "" {
		b.sendUIState()
		err := b.singlePlayer(b.playOnly)
		if err != nil && !errors.Is(err, exitPlaybackError) {
			return err
		}
		return QuitError
	}

	for {
		b.sendUIState()
		var cmd Command
		if b.nextCommand != nil {
			cmd = *b.nextCommand
			b.nextCommand = nil
		} else {
			cmd = <-b.Commands
		}
		b.uiState.Err = nil
		b.uiState.CurrentMessage = ""
		err := b.handleMainLoopCommand(cmd)
		if errors.Is(err, SigIntError) || errors.Is(err, QuitError) {
			return err
		} else if errors.Is(err, exitPlaybackError) {
			continue
		} else if err != nil {
			b.uiState.Err = err
			// Updated on next iteration.
		}
	}
	return nil
}

func (b *Backend) Close() {
	if b.nextOutPort != nil {
		b.nextOutPort = nil
	}
	if b.outPort != nil {
		b.outPort.Close()
		b.outPort = nil
	}
	close(b.UIStates)
}
