//go:build !wasm && !ios

package ebiplayer

func wakelockSet(goal bool) {}
