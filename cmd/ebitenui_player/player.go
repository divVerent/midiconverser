package main

import (
	"errors"
	"flag"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/divVerent/midiconverser/internal/ebiplayer"
	"github.com/divVerent/midiconverser/internal/player"
)

var (
	c    = flag.String("c", "midiconverser.yml", "config file name (YAML)")
	port = flag.String("port", "", "regular expression to match the preferred output port")
	i    = flag.String("i", "", "when set, just play this file then exit")
)

func Main() error {
	flag.Parse()

	w, h := 360, 800
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("MIDI Converser - graphical player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)

	var p ebiplayer.UI
	err := p.Init(w, h, *c, *i, *port)
	if err != nil {
		return err
	}

	defer p.Shutdown()
	return ebiten.RunGame(&p)
}

func main() {
	flag.Parse()
	err := Main()
	if errors.Is(err, player.SigIntError) {
		os.Exit(127)
	}
	if err != nil && !errors.Is(err, player.QuitError) {
		log.Printf("Exiting due to: %v.", err)
		os.Exit(1)
	}
}
