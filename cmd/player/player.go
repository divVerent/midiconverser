package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
	_ "gitlab.com/gomidi/midi/v2/drivers/webmididrv"
	"gitlab.com/gomidi/midi/v2/smf"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

var (
	c    = flag.String("c", "config.yml", "config file name (YAML)")
	i    = flag.String("i", "", "input file name (YAML)")
	port = flag.String("port", "", "regular expression to match the preferred output port")
)

var sigInt chan os.Signal

func sigSleep(t time.Duration) error {
	select {
	case <-sigInt:
		return fmt.Errorf("SIGINT caught")
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

// PreludePlayer plays the given file's whole verse for prelude purposes.
func PreludePlayer(config *processor.Config, optionsFile string) error {
	options, err := file.ReadOptions(optionsFile)
	if err != nil {
		return fmt.Errorf("failed to read %v: %v", optionsFile, err)
	}

	if options.NumVerses <= 1 {
		log.Printf("Skipping %s due to baked-in repeats.", optionsFile)
		return nil
	}

	output, err := file.Process(config, options)
	if err != nil {
		return fmt.Errorf("failed to process %v: %v", optionsFile, err)
	}

	verse := output[processor.OutputKey{Special: processor.Verse}]
	if verse == nil {
		return fmt.Errorf("no verse file for %v", optionsFile)
	}

	allOff := output[processor.OutputKey{Special: processor.Panic}]

	defer PlayMIDI(allOff)

	for i := 0; i < processor.WithDefault(config.PreludePlayerRepeat, 2); i++ {
		// TODO hook here: if cancel key was pressed, exit.
		err := PlayMIDI(verse)
		if err != nil {
			return fmt.Errorf("could not play %v: %v", optionsFile, err)
		}
		err = sigSleep(time.Duration(float64(time.Second) * processor.WithDefault(config.PreludePlayerSleepSec, 2.0)))
		if err != nil {
			return err
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
			return nil, fmt.Errorf("failed to compile -port RE %v: %v", *port, err)
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
		return fmt.Errorf("could not find MIDI port: %v", err)
	}
	log.Printf("Picked output port: %v", outPort)

	err = outPort.Open()
	if err != nil {
		return fmt.Errorf("could not open MIDI port %v: %v", outPort, err)
	}
	defer outPort.Close()

	config, err := file.ReadConfig(*c)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	return PreludePlayer(config, *i)
}

func main() {
	flag.Parse()
	err := Main()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
