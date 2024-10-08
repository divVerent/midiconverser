package player

import (
	"fmt"
	"regexp"
	"slices"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

var (
	badPortsRE       = regexp.MustCompile(`\bMidi Through\b|\bPipeWire-System\b|\bPipeWire-RT-Event\b`)
	usbPortsRE       = regexp.MustCompile(`\bUSB|\bUM-`)
	softSynthPortsRE = regexp.MustCompile(`\bFLUID\b|\bSynth\b|\bTiMidity\b`)
)

func FindBestPort(pattern string, preferred string) (drivers.Out, error) {
	var goodPorts []drivers.Out
	if pattern != "" {
		portRE, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile -port RE %v: %w", pattern, err)
		}
		for _, port := range midi.GetOutPorts() {
			if !portRE.MatchString(port.String()) {
				continue
			}
			goodPorts = append(goodPorts, port)
		}
	}
	if len(goodPorts) == 0 && preferred != "" {
		for _, port := range midi.GetOutPorts() {
			if port.String() != preferred {
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
