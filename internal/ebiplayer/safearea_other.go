//go:build !ios

package ebiplayer

import (
	"github.com/ebitenui/ebitenui/widget"
)

func safeAreaMargins() widget.Insets {
	return widget.Insets{}
}
