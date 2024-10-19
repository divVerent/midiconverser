//go:build wasm

package ebiplayer

import (
	"syscall/js"
)

// wakelockGoal exists to avoid redundant calls into JS.
var wakelockGoal = false

func wakelockSet(goal bool) {
	if goal == wakelockGoal {
		return
	}
	js.Global().Call("wakelockSet", goal)
	wakelockGoal = goal
}
