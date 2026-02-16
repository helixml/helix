package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goSystrayStart(void);

static void dispatchSystrayOnMain() {
	dispatch_async(dispatch_get_main_queue(), ^{
		goSystrayStart();
	});
}

// fixStatusItemIconSize accesses energye/systray's internal SystrayAppDelegate
// via the Obj-C runtime and resizes the icon to the standard macOS menu bar
// size (18x18 points). The library hardcodes NSMakeSize(16, 16) in setIcon().
static void fixStatusItemIconSize() {
	dispatch_async(dispatch_get_main_queue(), ^{
		// 'owner' is a file-scope global in systray_darwin.m (external linkage)
		extern NSObject *owner;
		if (!owner) return;
		Ivar ivar = class_getInstanceVariable([owner class], "statusItem");
		if (!ivar) return;
		NSStatusItem *item = object_getIvar(owner, ivar);
		if (item && item.button && item.button.image) {
			[item.button.image setSize:NSMakeSize(18, 18)];
		}
	});
}
*/
import "C"

var pendingSystrayStart func()

//export goSystrayStart
func goSystrayStart() {
	if pendingSystrayStart != nil {
		pendingSystrayStart()
	}
}

// startSystrayOnMainThread dispatches the systray start function to the main
// thread via GCD. NSStatusItem (used by energye/systray) must be created on
// the main thread, but Wails' startup callback runs on a background goroutine.
func startSystrayOnMainThread(start func()) {
	pendingSystrayStart = start
	C.dispatchSystrayOnMain()
}

// fixTrayIconSize overrides energye/systray's hardcoded 16x16 icon size
// to the standard macOS menu bar size (18x18 points).
func fixTrayIconSize() {
	C.fixStatusItemIconSize()
}
