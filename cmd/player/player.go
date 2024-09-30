package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
	_ "gitlab.com/gomidi/midi/v2/drivers/webmididrv"
	"gitlab.com/gomidi/midi/v2/smf"
	"golang.org/x/term"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

type tagList []string

func (l tagList) String() string {
	return strings.Join([]string(l), " ")
}

func (l *tagList) Set(s string) error {
	*l = strings.Split(s, " ")
	return nil
}

func noTagsDefault() tagList {
	noTags := tagList{"noprelude", "national"}
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
	log.Printf("First Advent: %v", beginTime)
	log.Printf("Xmas ends: %v", endTime)
	if now.Before(beginTime) || !now.Before(endTime) {
		noTags = append(noTags, "xmas")
	}
	return noTags
}

var (
	c        = flag.String("c", "config.yml", "config file name (YAML)")
	port     = flag.String("port", "", "regular expression to match the preferred output port")
	wantTags tagList
	noTags   tagList
)

func init() {
	flag.Var(&wantTags, "want_tags", "list of tags any of which must be in each prelude hymn")
	noTags = noTagsDefault()
	flag.Var(&noTags, "no_tags", "list of tags all of which must not be in each prelude hymn")
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

	// Tempo sets the tempo to a new factor.
	Tempo float64

	// NumVerses is an override for the verse count.
	NumVerses int

	// Answer continues the current playback (exits a Prompt state).
	Answer bool
}

func (c Command) IsZero() bool {
	return c == Command{}
}

func (c Command) IsMainLoopCommand() bool {
	return c.Exit || c.Quit || c.PlayOne != "" || c.PlayPrelude || c.IsZero()
}

type UIState struct {
	// Err is set to show an error message. The backend is dead, but can be
	// restarted by sending a PlayOne or PlayPrelude message.
	Err error

	// These correspond to the Commands.

	// PlayOne is the name of the currently playing file.
	PlayOne string

	// PlayPrelude indicates if we're in the prelude player.
	PlayPrelude bool

	// Tempo is the current tempo as a factor of normal.
	Tempo float64

	// Number of verses to play.
	NumVerses int

	// Prompt is the text to prompt the user with.
	// To clear a prompt, send the Answer message.
	Prompt string

	// Informational outputs.

	// CurrentFile is the currently being played file.
	CurrentFile string

	// CurrentMessage is a message for what is currently playing.
	CurrentMessage string

	// Playing is whether we are currently playing.
	Playing bool

	// PlaybackPos is the current playback position.
	PlaybackPos time.Duration

	// PlaybackLen is the length of the current file.
	PlaybackLen time.Duration

	// Verse is the current verse.
	Verse int
}

type Backend struct {
	// Commands can be used to send commands to the UI.
	Commands chan Command

	// UIStates receives updates to the UI state non-blockingly.
	UIStates chan UIState

	// The configuration data.
	config processor.Config

	// port is the MIDI port to play to.
	outPort drivers.Out

	// The current UI state. Sent to the client on every update, nonblockingly.
	uiState UIState

	// The next command to be executed. Commands that cannot be processed
	// immediately are enqueued here and handled by the main loop. There can be
	// only one.
	nextCommand *Command
}

func NewBackend(config *processor.Config, outPort drivers.Out) *Backend {
	return &Backend{
		Commands: make(chan Command, 10),
		UIStates: make(chan UIState, 10),
		config:   *config,
		outPort:  outPort,
		uiState: UIState{
			Tempo:          1.0,
			CurrentMessage: "initializing player",
		},
		nextCommand: nil,
	}
}

func (b *Backend) sendUIState() {
	select {
	case b.UIStates <- b.uiState:
		return
	default:
		log.Printf("tried to send an UI state, but nobody came")
		return
	}
}

var sigIntError = errors.New("SIGINT caught")
var sigInt = make(chan os.Signal, 1)

func init() {
	signal.Notify(sigInt, os.Interrupt)
}

func (b *Backend) sigSleep(t time.Duration) error {
	done := time.After(t)
	for {
		select {
		case <-sigInt:
			return sigIntError
		case cmd := <-b.Commands:
			if err := b.handleCommandDuringSleep(cmd); err != nil {
				if !errors.Is(err, promptAnsweredError) {
					if b.nextCommand != nil {
						log.Panicf("already have a nextCommand")
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

func (b *Backend) handleCommandDuringSleep(cmd Command) error {
	if cmd.IsMainLoopCommand() {
		return exitPlaybackError
	}
	switch {
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
			log.Printf("Spurious prompt answer: %+v", cmd)
			return nil
		}
		return promptAnsweredError // Caught when waiting for prompt.
	default:
		return fmt.Errorf("unrecognized command: %+v", cmd)
	}
}

var outPort drivers.Out

// playMIDI plays a MIDI file on the current thread.
func (b *Backend) playMIDI(mid *smf.SMF) error {
	var maxTick int64
	err := processor.ForEachEventWithTime(mid, func(t int64, track int, msg smf.Message) error {
		maxTick = t
		return nil
	})
	maxT := time.Microsecond * time.Duration(mid.TimeAt(maxTick))
	b.uiState.Playing = true
	b.uiState.Err = nil
	b.uiState.PlaybackLen = maxT
	b.uiState.PlaybackPos = 0
	b.sendUIState()

	defer func() {
		b.uiState.Playing = false
		b.sendUIState()
	}()

	var prevT time.Duration
	prevNow := time.Now()

	err = processor.ForEachEventWithTime(mid, func(t int64, track int, msg smf.Message) error {
		if msg.IsMeta() {
			return nil
		}

		// Parse MIDI.
		midiT := time.Microsecond * time.Duration(mid.TimeAt(t))
		midiMsg := midi.Message(msg)

		// Back to delta.
		deltaT := midiT - prevT
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
		b.sendUIState()

		// Write to output.
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

// load loads and processes the given input.
func load(config *processor.Config, optionsFile string) (map[processor.OutputKey]*smf.SMF, *processor.Options, error) {
	options, err := file.ReadOptions(optionsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %v: %w", optionsFile, err)
	}

	output, err := file.Process(config, options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process %v: %w", optionsFile, err)
	}

	err = fixOutput(output)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to autofix %v: %w", optionsFile, err)
	}

	return output, options, nil
}

// preludePlayerOne plays the given file's whole verse for prelude purposes.
func (b *Backend) preludePlayerOne(optionsFile string) error {
	output, options, err := load(&b.config, optionsFile)
	if err != nil {
		return err
	}

	if options.NumVerses <= 1 {
		log.Printf("Skipping %s due to baked-in repeats.", optionsFile)
		return nil
	}

	allOff := output[processor.OutputKey{Special: processor.Panic}]
	defer b.playMIDI(allOff)

	verse := output[processor.OutputKey{Special: processor.Verse}]
	if verse == nil {
		return fmt.Errorf("no verse file for %v", optionsFile)
	}

	log.Printf("Playing full verses for prelude: %v", optionsFile)
	for i := 0; i < processor.WithDefault(b.config.PreludePlayerRepeat, 2); i++ {
		err := b.playMIDI(verse)
		if err != nil {
			return fmt.Errorf("could not play %v: %w", optionsFile, err)
		}
		err = b.sigSleep(time.Duration(float64(time.Second) * processor.WithDefault(b.config.PreludePlayerSleepSec, 2.0)))
		if err != nil {
			return err
		}
	}
	return nil
}

// preludePlayer plays random hymns.
func (b *Backend) preludePlayer() error {
	for {
		all, err := filepath.Glob("*.yml")
		if err != nil {
			return fmt.Errorf("glob: %w", err)
		}
		rand.Shuffle(len(all), func(i, j int) {
			all[i], all[j] = all[j], all[i]
		})
		gotOne := false
		for _, f := range all {
			if f == *c {
				continue
			}
			err := b.preludePlayerOne(f)
			if err != nil {
				return err
			}
			gotOne = true
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
	output, options, err := load(&b.config, optionsFile)
	if err != nil {
		return err
	}

	allOff := output[processor.OutputKey{Special: processor.Panic}]
	defer b.playMIDI(allOff)

	log.Printf("Playing all verses of %v", optionsFile)

	n := processor.WithDefault(options.NumVerses, 1)
	b.uiState.CurrentFile = optionsFile
	b.uiState.NumVerses = n
	b.sendUIState()
	defer func() {
		b.uiState.CurrentFile = ""
		b.sendUIState()
	}()

	prelude := output[processor.OutputKey{Special: processor.Prelude}]
	if prelude != nil {
		err := b.prompt("start prelude", "playing prelude")
		if err != nil {
			return err
		}
		err = b.playMIDI(prelude)
		if err != nil {
			return fmt.Errorf("could not play %v prelude: %w", optionsFile, err)
		}
	}

	for i := 0; i < b.uiState.NumVerses; i++ {
		b.uiState.Verse = i
		for j := 0; ; j++ {
			part := output[processor.OutputKey{Part: j}]
			if part == nil {
				break
			}
			var msg string
			if j == 0 {
				msg = "start verse"
			} else if j%2 == 1 {
				msg = "end fermata"
			} else {
				msg = "continue"
			}
			err := b.prompt(msg, "playing verse")
			if err != nil {
				return err
			}
			err = b.playMIDI(part)
			if err != nil {
				return fmt.Errorf("could not play %v part %v: %w", optionsFile, j, err)
			}
		}
	}

	postlude := output[processor.OutputKey{Special: processor.Postlude}]
	if postlude != nil {
		err := b.prompt("postlude", "playing postlude")
		if err != nil {
			return err
		}
		err = b.playMIDI(postlude)
		if err != nil {
			return fmt.Errorf("could not play %v postlude: %w", optionsFile, err)
		}
	}

	return nil
}

var quitError = errors.New("intentionally quitting")

func (b *Backend) handleMainLoopCommand(cmd Command) error {
	if !cmd.IsMainLoopCommand() {
		return b.handleCommandDuringSleep(cmd)
	}
	switch {
	case cmd.Exit:
		return nil
	case cmd.Quit:
		return quitError
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
	defer close(b.UIStates)
	b.sendUIState()
	for {
		var cmd Command
		if b.nextCommand != nil {
			cmd = *b.nextCommand
			b.nextCommand = nil
		} else {
			cmd = <-b.Commands
		}
		log.Printf("running %v", cmd)
		err := b.handleMainLoopCommand(cmd)
		if errors.Is(err, sigIntError) || errors.Is(err, quitError) {
			return err
		} else if errors.Is(err, exitPlaybackError) {
			continue
		} else if err != nil {
			b.uiState.Err = err
			b.sendUIState()
		}
	}
	return nil
}

var (
	badPortsRE       = regexp.MustCompile(`\bMidi Through\b`)
	usbPortsRE       = regexp.MustCompile(`\bUSB|\bUM-`)
	softSynthPortsRE = regexp.MustCompile(`\bFLUID\b|\bTiMidity\b`)
)

func findBestPort() (drivers.Out, error) {
	var goodPorts []drivers.Out
	if *port != "" {
		portRE, err := regexp.Compile(*port)
		if err != nil {
			return nil, fmt.Errorf("failed to compile -port RE %v: %w", *port, err)
		}
		for _, port := range midi.GetOutPorts() {
			if !portRE.MatchString(port.String()) {
				continue
			}
			goodPorts = append(goodPorts, port)
		}
	}
	if len(goodPorts) == 0 {
		for _, port := range midi.GetOutPorts() {
			if badPortsRE.MatchString(port.String()) {
				continue
			}
			goodPorts = append(goodPorts, port)
		}
	}
	if len(goodPorts) == 0 {
		return nil, fmt.Errorf("no selected port found")
	}
	return slices.MinFunc(goodPorts, func(a, b drivers.Out) int {
		aUSB := usbPortsRE.MatchString(a.String())
		bUSB := usbPortsRE.MatchString(b.String())
		if aUSB != bUSB {
			// Prefer USB.
			if aUSB {
				return -1
			}
			return 1
		}
		aSoftSynth := softSynthPortsRE.MatchString(a.String())
		bSoftSynth := softSynthPortsRE.MatchString(b.String())
		if aSoftSynth != bSoftSynth {
			// Avoid software synthesizers.
			if aSoftSynth {
				return 1
			}
			return -1
		}
		// Otherwise sort arbitrarily.
		return a.Number() - b.Number()
	}), nil
}

var (
	preludeRE = regexp.MustCompile(`^prelude$`)
	playRE    = regexp.MustCompile(`^play (\S+)$`)
	tempoRE   = regexp.MustCompile(`^tempo ([\d.]+)$`)
	versesRE  = regexp.MustCompile(`^verses (\d+)$`)
	quitRE    = regexp.MustCompile(`^q(?:u(?:it?)?)?$`)
)

func processCommand(b *Backend, cmd []byte) error {
	if preludeRE.Match(cmd) {
		b.Commands <- Command{
			PlayPrelude: true,
		}
		return nil
	}
	if sub := playRE.FindSubmatch(cmd); sub != nil {
		b.Commands <- Command{
			PlayOne: string(sub[1]),
		}
		return nil
	}
	if sub := tempoRE.FindSubmatch(cmd); sub != nil {
		num := 0.0
		_, err := fmt.Sscanf(string(sub[1]), "%f", &num)
		if err != nil {
			return errors.New("failed to parse command: does not end with a number")
		}
		if num <= 0 {
			return errors.New("tempo must be positive")
		}
		b.Commands <- Command{
			Tempo: num,
		}
		return nil
	}
	if sub := versesRE.FindSubmatch(cmd); sub != nil {
		num := 0
		_, err := fmt.Sscanf(string(sub[1]), "%d", &num)
		if err != nil {
			return errors.New("failed to parse command: does not end with an integer")
		}
		if num < 1 {
			return errors.New("verse count must be positive")
		}
		b.Commands <- Command{
			NumVerses: num,
		}
		return nil
	}
	if quitRE.Match(cmd) {
		b.Commands <- Command{
			Quit: true,
		}
		return nil
	}
	return errors.New("unknown command")
}

func textModeUI(b *Backend) error {
	defer close(b.Commands) // This will invariably cause failure when reading.

	stdinFD := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(stdinFD)
	if err != nil {
		return fmt.Errorf("cannot make terminal raw: %v", err)
	}
	defer term.Restore(stdinFD, oldState)

	stdin := make(chan byte)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				log.Printf("error reading stdin: %v", err)
				close(stdin)
				return
			}
			if n == 0 {
				continue
			}
			stdin <- buf[0]
		}
	}()

	var ui UIState
	var ok bool
	inputMode := true
	var inputCommand []byte

	for {
		inputModePrompt := ""
		if inputMode {
			inputModePrompt = ":" + string(inputCommand)
		}
		fmt.Fprintf(os.Stderr, "\r\n\r\n%+v\r\n%s", ui, inputModePrompt)
		select {
		case ui, ok = <-b.UIStates:
			if !ok {
				// UI channel was closed.
				return nil
			}
			// Rest handled below.
		case ch := <-stdin:
			if inputMode {
				switch ch {
				case 0x08, 0x7F:
					if len(inputCommand) > 0 {
						inputCommand = inputCommand[:len(inputCommand)-1]
					}
				case 0x0A, 0x0D:
					if len(inputCommand) > 0 {
						err := processCommand(b, inputCommand)
						if err != nil {
							fmt.Printf("could not parse command %q: %v", inputCommand, err)
						}
					}
					inputCommand = inputCommand[:0]
					inputMode = false
				case 0x1B:
					inputMode = false
				default:
					if ch == ':' && len(inputCommand) == 0 {
						continue
					}
					inputCommand = append(inputCommand, ch)
				}
			} else {
				switch ch {
				case '+', '=', '.':
					// More tempo.
					t := ui.Tempo + 0.01
					if t > 2 {
						t = 2
					}
					b.Commands <- Command{
						Tempo: t,
					}
				case '-', '_', ',':
					// Less tempo.
					t := ui.Tempo - 0.01
					if t < 0.5 {
						t = 0.5
					}
					b.Commands <- Command{
						Tempo: t,
					}
				case 0x03:
					// Ctrl-C. Quit right away.
					b.Commands <- Command{
						Quit: true,
					}
				case 0x1B:
					// Exit to input mode.
					b.Commands <- Command{
						Exit: true,
					}
					inputMode = true
				case ':':
					// Input mode during playback.
					inputMode = true
				default:
					// "Any key".
					if ui.Prompt != "" {
						b.Commands <- Command{
							Answer: true,
						}
					}
				}
			}
		}
	}
	return nil
}

func Main() error {
	var err error
	outPort, err = findBestPort()
	if err != nil {
		return fmt.Errorf("could not find MIDI port: %w", err)
	}
	log.Printf("Picked output port: %v", outPort)

	err = outPort.Open()
	if err != nil {
		return fmt.Errorf("could not open MIDI port %v: %w", outPort, err)
	}
	defer outPort.Close()

	config, err := file.ReadConfig(*c)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	b := NewBackend(config, outPort)

	go func() {
		b.Loop()
	}()

	return textModeUI(b)
}

func main() {
	flag.Parse()
	err := Main()
	if errors.Is(err, sigIntError) {
		os.Exit(127)
	}
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
