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
	i        = flag.String("i", "", "input file name (YAML)")
	port     = flag.String("port", "", "regular expression to match the preferred output port")
	wantTags tagList
	noTags   tagList
)

func init() {
	flag.Var(&wantTags, "want_tags", "list of tags any of which must be in each prelude hymn")
	noTags = noTagsDefault()
	flag.Var(&noTags, "no_tags", "list of tags all of which must not be in each prelude hymn")
}

var sigIntError = errors.New("SIGINT caught")

var sigInt chan os.Signal

func sigSleep(t time.Duration) error {
	select {
	case <-sigInt:
		return sigIntError
	// TODO hook here: button to cancel playback.
	case <-time.After(t):
		return nil
	}
}

var outPort drivers.Out

// PlayMIDI plays a MIDI file on the current thread.
func PlayMIDI(mid *smf.SMF) error {
	var prevT time.Duration
	prevNow := time.Now()
	return processor.ForEachEventWithTime(mid, func(t int64, track int, msg smf.Message) error {
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
		newNow := prevNow.Add(deltaT) // TODO hook here: allow changing tempo.
		waitTime := newNow.Sub(time.Now())
		if waitTime > 0 {
			err := sigSleep(waitTime)
			if err != nil {
				return err
			}
		}
		prevNow = newNow

		// Write to output.
		return outPort.Send(midiMsg)
	})
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

// Load loads and processes the given input.
func Load(config *processor.Config, optionsFile string) (map[processor.OutputKey]*smf.SMF, *processor.Options, error) {
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

// PreludePlayerOne plays the given file's whole verse for prelude purposes.
func PreludePlayerOne(config *processor.Config, optionsFile string) error {
	output, options, err := Load(config, optionsFile)
	if err != nil {
		return err
	}

	if options.NumVerses <= 1 {
		log.Printf("Skipping %s due to baked-in repeats.", optionsFile)
		return nil
	}

	allOff := output[processor.OutputKey{Special: processor.Panic}]
	defer PlayMIDI(allOff)

	verse := output[processor.OutputKey{Special: processor.Verse}]
	if verse == nil {
		return fmt.Errorf("no verse file for %v", optionsFile)
	}

	log.Printf("Playing full verses for prelude: %v", optionsFile)
	for i := 0; i < processor.WithDefault(config.PreludePlayerRepeat, 2); i++ {
		err := PlayMIDI(verse)
		if err != nil {
			return fmt.Errorf("could not play %v: %w", optionsFile, err)
		}
		err = sigSleep(time.Duration(float64(time.Second) * processor.WithDefault(config.PreludePlayerSleepSec, 2.0)))
		if err != nil {
			return err
		}
	}
	return nil
}

// PreludePlayer plays random hymns.
func PreludePlayer(config *processor.Config) error {
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
			err := PreludePlayerOne(config, f)
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

func prompt(ask, response string) error {
	// TODO ebitenify.
	fmt.Printf("\n\n\n%v\nPress any key...\n", ask)
	buf, err := func() ([]byte, error) {
		save, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return nil, err
		}
		defer term.Restore(int(os.Stdin.Fd()), save)
		buf := make([]byte, 1)
		_, err = os.Stdin.Read(buf)
		return buf, err
	}()
	if err != nil {
		return err
	}
	if buf[0] == 0x03 {
		return sigIntError
	}
	fmt.Printf("%v\n", response)
	return nil
}

// SinglePlayer plays the given file interactively.
func SinglePlayer(config *processor.Config, optionsFile string) error {
	output, options, err := Load(config, optionsFile)
	if err != nil {
		return err
	}

	allOff := output[processor.OutputKey{Special: processor.Panic}]
	defer PlayMIDI(allOff)

	log.Printf("Playing all verses of %v", optionsFile)

	prelude := output[processor.OutputKey{Special: processor.Prelude}]
	if prelude != nil {
		err := prompt("start prelude", "playing prelude")
		if err != nil {
			return err
		}
		err = PlayMIDI(prelude)
		if err != nil {
			return fmt.Errorf("could not play %v prelude: %w", optionsFile, err)
		}
	}

	n := processor.WithDefault(options.NumVerses, 1)
	for i := 0; i < n; i++ {
		verseStr := fmt.Sprintf("verse %d/%d", i+1, n)
		for j := 0; ; j++ {
			part := output[processor.OutputKey{Part: j}]
			if part == nil {
				break
			}
			var msg string
			if j == 0 {
				msg = fmt.Sprintf("start %v", verseStr)
			} else if j%2 == 1 {
				msg = fmt.Sprintf("end %v fermata", verseStr)
			} else {
				msg = fmt.Sprintf("continue %v", verseStr)
			}
			err := prompt(msg, fmt.Sprintf("playing %v", verseStr))
			if err != nil {
				return err
			}
			err = PlayMIDI(part)
			if err != nil {
				return fmt.Errorf("could not play %v part %v: %w", optionsFile, j, err)
			}
		}
	}

	postlude := output[processor.OutputKey{Special: processor.Postlude}]
	if postlude != nil {
		err := prompt("postlude", "playing postlude")
		if err != nil {
			return err
		}
		err = PlayMIDI(postlude)
		if err != nil {
			return fmt.Errorf("could not play %v postlude: %w", optionsFile, err)
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

func Main() error {
	sigInt = make(chan os.Signal, 1)
	signal.Notify(sigInt, os.Interrupt)

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

	if *i == "" {
		return PreludePlayer(config)
	}

	return SinglePlayer(config, *i)
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
