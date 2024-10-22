//go:build ios

package ebiplayer

/*
#cgo CFLAGS: -x objective-c

#import <UIKit/UIApplication.h>

void wakelockSet(bool goal) {
	// Always have to set it to NO first:
	// https://stackoverflow.com/questions/1058717/idletimerdisabled-not-working-since-iphone-3-0
	[[UIApplication sharedApplication] setIdleTimerDisabled:NO];
	if (goal) {
		[[UIApplication sharedApplication] setIdleTimerDisabled:YES];
	}
}
*/
import "C"

func wakelockSet(goal bool) {
	C.wakelockSet(C.bool(goal))
}
