//go:build wasm

package main

import (
	"syscall/js"
)

func rescanMIDI() {
	// Sadly we cannot rescan MIDI devices in the HTML5 interface.
	// Reload the page then.
	js.Global().Get("location").Call("reload")
}
