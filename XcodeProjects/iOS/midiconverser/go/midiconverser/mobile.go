//go:build ios
// +build ios

package midiconverser

import (
	"github.com/hajimehoshi/ebiten/v2/mobile"

	"github.com/divVerent/midiconverser/cmd/ebitenui_player/game"
)

var (
	g *game.Game
)

func init() {
	g = &game.Game{}
	mobile.SetGame(g)
}

// Dummy is an exported name to make ebitenmobile happy. It does nothing.
func Dummy() {
}
