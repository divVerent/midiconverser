package ebiplayer

import (
	"time"
)

var (
	wakelockPrevGoal bool
	wakelockNextTime time.Time
)

func wakelockSet(goal bool) {
	now := time.Now()
	if goal == wakelockPrevGoal && now.Before(wakelockNextTime) {
		return
	}
	wakelockSetInternal(goal)
	wakelockPrevGoal = goal
	wakelockNextTime = now.Add(wakelockRefreshInterval)
}
