package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

static void setCursorByName(const char* name) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSString *n = [NSString stringWithUTF8String:name];
		NSCursor *cursor = nil;

		if ([n isEqualToString:@"col-resize"] || [n isEqualToString:@"ew-resize"]) {
			cursor = [NSCursor resizeLeftRightCursor];
		} else if ([n isEqualToString:@"row-resize"] || [n isEqualToString:@"ns-resize"]) {
			cursor = [NSCursor resizeUpDownCursor];
		} else if ([n isEqualToString:@"pointer"]) {
			cursor = [NSCursor pointingHandCursor];
		} else if ([n isEqualToString:@"text"]) {
			cursor = [NSCursor IBeamCursor];
		} else if ([n isEqualToString:@"crosshair"]) {
			cursor = [NSCursor crosshairCursor];
		} else if ([n isEqualToString:@"grab"] || [n isEqualToString:@"move"]) {
			cursor = [NSCursor openHandCursor];
		} else if ([n isEqualToString:@"grabbing"]) {
			cursor = [NSCursor closedHandCursor];
		} else if ([n isEqualToString:@"not-allowed"] || [n isEqualToString:@"no-drop"]) {
			cursor = [NSCursor operationNotAllowedCursor];
		} else if ([n isEqualToString:@"copy"]) {
			cursor = [NSCursor dragCopyCursor];
		} else if ([n isEqualToString:@"context-menu"]) {
			cursor = [NSCursor contextualMenuCursor];
		} else {
			cursor = [NSCursor arrowCursor];
		}

		[cursor set];
	});
}
*/
import "C"

import "unsafe"

// SetCursor sets the native macOS cursor by CSS cursor name.
// Called from the frontend via the WKWebView cursor bridge.
func (a *App) SetCursor(name string) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	C.setCursorByName(cName)
}
