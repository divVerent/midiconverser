//go:build wasm

package ebiplayer

import (
	"time"
	"syscall/js"
)

const wakelockRefreshInterval = time.Second * 10

func wakelockSet(goal bool) {
	js.Global().Call("wakelockSet", goal)
}
