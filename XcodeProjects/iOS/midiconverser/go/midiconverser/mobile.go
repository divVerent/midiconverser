//go:build ios
// +build ios

package midiconverser

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2/mobile"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/divVerent/midiconverser/internal/ebiplayer"
)

type game struct {
	ui ebiplayer.UI
	drawErr error
}

func (g *game) Update() (err error) {
	ok := false
	defer func() {
		if !ok {
			err = fmt.Errorf("caught panic during update: %v", recover())
		}
		if err != nil {
			// Make sure to always stop all MIDI notes.
			g.ui.Shutdown()
		}
	}()
	if g.drawErr != nil {
		ok = true
		return g.drawErr
	}
	err = g.ui.Update()
	ok = true
	return err
}

func (g *game) Draw(screen *ebiten.Image) {
	ok := false
	defer func() {
		if !ok {
			g.drawErr = fmt.Errorf("caught panic during draw: %v", recover())
		}
	}()
	g.ui.Draw(screen)
	ok = true
}

func (g *game) Layout(w, h int) (int, int) {
	return g.ui.Layout(w, h)
}

func init() {
	var g game
	err := g.ui.Init(360, 800, "midiconverser.yml", "", "")
	if err != nil {
		panic(err)
	}
	mobile.SetGame(&g)
}

// Dummy is an exported name to make ebitenmobile happy. It does nothing.
func Dummy() {
}
