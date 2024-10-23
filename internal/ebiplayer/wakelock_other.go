//go:build !wasm && !ios

package ebiplayer

import (
	"time"
)

const wakelockRefreshInterval = time.Hour

func wakelockSet(goal bool) {}
