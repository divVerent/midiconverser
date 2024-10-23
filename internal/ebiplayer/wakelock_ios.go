//go:build ios

package ebiplayer

import (
	"time"
)

/*
#cgo CFLAGS: -x objective-c

#import <UIKit/UIApplication.h>

#include <dispatch/dispatch.h>

void wakelockSet(bool goal) {
	if (goal) {
		dispatch_async(dispatch_get_main_queue(), ^{
			// Always have to set it to NO first:
			// https://stackoverflow.com/questions/1058717/idletimerdisabled-not-working-since-iphone-3-0
			[[UIApplication sharedApplication] setIdleTimerDisabled:NO];
			[[UIApplication sharedApplication] setIdleTimerDisabled:YES];
		});
	} else {
		dispatch_async(dispatch_get_main_queue(), ^{
			[[UIApplication sharedApplication] setIdleTimerDisabled:NO];
		});
	}
}
*/
import "C"

const wakelockRefreshInterval = time.Seconds * 10

func wakelockSetInternal(goal bool) {
	C.wakelockSet(C.bool(goal))
}
