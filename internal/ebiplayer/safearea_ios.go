//go:build ios

package ebiplayer

import (
	"log"

	"github.com/ebitenui/ebitenui/widget"
)

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation

#import <UIKit/UIApplication.h>
#import <UIKit/UIWindow.h>

bool insets(int *left, int *top, int *right, int *bottom) {
	UIWindow *window = [UIApplication sharedApplication].keyWindow;
	if (window == NULL) {
		return false;
	}
	*left = window.safeAreaInsets.left;
	*top = window.safeAreaInsets.top;
	*right = window.safeAreaInsets.right;
	*bottom = window.safeAreaInsets.bottom;
	return true;
}
*/
import "C"

func safeAreaMargins() widget.Insets {
	var left, top, right, bottom C.int
	if !C.insets(&left, &top, &right, &bottom) {
		log.Printf("Insets not found; maybe next frame?")
		return widget.Insets{}
	}
	return widget.Insets{
		Left:   int(left),
		Top:    int(top),
		Right:  int(right),
		Bottom: int(bottom),
	}
}
