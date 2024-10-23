//go:build !wasm && !ios

package ebiplayer

import (
	"time"
)

const wakelockRefreshInterval = time.Hour

func wakelockSetInternal(goal bool) {}
