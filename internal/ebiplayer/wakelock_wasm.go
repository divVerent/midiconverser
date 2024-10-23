//go:build wasm

package ebiplayer

import (
	"syscall/js"
	"time"
)

const wakelockRefreshInterval = time.Second * 10

func wakelockSetInternal(goal bool) {
	js.Global().Call("wakelockSet", goal)
}
