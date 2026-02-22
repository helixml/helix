package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// The desired cursor, updated by JS via SetCursor. The mouse-moved monitor
// re-applies it on every move so WKWebView's tracking areas can't override it.
static NSCursor *_desiredCursor = nil;
static id _cursorMonitor = nil;

// Cached custom diagonal resize cursors (created once, reused)
static NSCursor *_neswResizeCursor = nil;
static NSCursor *_nwseResizeCursor = nil;

// Create a diagonal resize cursor by drawing a double-headed arrow.
// angle is in degrees: 45 = NE-SW, 135 = NW-SE
static NSCursor* createDiagonalResizeCursor(CGFloat angle) {
	CGFloat size = 22;
	CGFloat arrowLen = 8;
	CGFloat arrowAngle = 30;
	CGFloat lineLen = 16;
	NSImage *img = [[NSImage alloc] initWithSize:NSMakeSize(size, size)];
	[img lockFocus];

	NSAffineTransform *t = [NSAffineTransform transform];
	[t translateXBy:size/2 yBy:size/2];
	[t rotateByDegrees:-angle];
	[t concat];

	for (int pass = 0; pass < 2; pass++) {
		CGFloat w = pass == 0 ? 3.0 : 1.5;
		NSColor *c = pass == 0 ? [NSColor whiteColor] : [NSColor blackColor];
		[c setStroke];

		NSBezierPath *path = [NSBezierPath bezierPath];
		[path setLineWidth:w];
		[path setLineCapStyle:NSLineCapStyleRound];
		[path setLineJoinStyle:NSLineJoinStyleRound];

		// Shaft
		[path moveToPoint:NSMakePoint(-lineLen/2, 0)];
		[path lineToPoint:NSMakePoint(lineLen/2, 0)];

		// Arrowhead at positive end
		CGFloat ax = arrowLen * cos(arrowAngle * M_PI / 180);
		CGFloat ay = arrowLen * sin(arrowAngle * M_PI / 180);
		[path moveToPoint:NSMakePoint(lineLen/2 - ax, ay)];
		[path lineToPoint:NSMakePoint(lineLen/2, 0)];
		[path lineToPoint:NSMakePoint(lineLen/2 - ax, -ay)];

		// Arrowhead at negative end
		[path moveToPoint:NSMakePoint(-lineLen/2 + ax, ay)];
		[path lineToPoint:NSMakePoint(-lineLen/2, 0)];
		[path lineToPoint:NSMakePoint(-lineLen/2 + ax, -ay)];

		[path stroke];
	}

	[img unlockFocus];
	return [[NSCursor alloc] initWithImage:img hotSpot:NSMakePoint(size/2, size/2)];
}

static NSCursor* neswResizeCursor(void) {
	if (!_neswResizeCursor) _neswResizeCursor = createDiagonalResizeCursor(135);
	return _neswResizeCursor;
}

static NSCursor* nwseResizeCursor(void) {
	if (!_nwseResizeCursor) _nwseResizeCursor = createDiagonalResizeCursor(45);
	return _nwseResizeCursor;
}

// cursorForName maps CSS cursor names to NSCursor objects.
// The set of names matches what the Helix desktop-bridge sends from Ubuntu
// (see api/pkg/desktop/cursor_sprites.go for the canonical list).
// Returns nil for cursors WKWebView handles correctly on its own.
static NSCursor* cursorForName(NSString *n) {
	// Resize cursors — these are the ones WKWebView breaks
	if ([n isEqualToString:@"ew-resize"] || [n isEqualToString:@"col-resize"])
		return [NSCursor resizeLeftRightCursor];
	if ([n isEqualToString:@"ns-resize"] || [n isEqualToString:@"row-resize"])
		return [NSCursor resizeUpDownCursor];
	if ([n isEqualToString:@"nesw-resize"])
		return neswResizeCursor();
	if ([n isEqualToString:@"nwse-resize"])
		return nwseResizeCursor();

	// Other cursors that WKWebView may not render in cross-origin iframes
	if ([n isEqualToString:@"pointer"] || [n isEqualToString:@"hand"])
		return [NSCursor pointingHandCursor];
	if ([n isEqualToString:@"text"] || [n isEqualToString:@"ibeam"])
		return [NSCursor IBeamCursor];
	if ([n isEqualToString:@"crosshair"] || [n isEqualToString:@"cross"] || [n isEqualToString:@"cell"])
		return [NSCursor crosshairCursor];
	if ([n isEqualToString:@"move"] || [n isEqualToString:@"all-scroll"] ||
	    [n isEqualToString:@"grab"] || [n isEqualToString:@"openhand"])
		return [NSCursor openHandCursor];
	if ([n isEqualToString:@"grabbing"] || [n isEqualToString:@"closedhand"])
		return [NSCursor closedHandCursor];
	if ([n isEqualToString:@"not-allowed"] || [n isEqualToString:@"no-drop"])
		return [NSCursor operationNotAllowedCursor];
	if ([n isEqualToString:@"copy"])
		return [NSCursor dragCopyCursor];
	if ([n isEqualToString:@"alias"])
		return [NSCursor dragLinkCursor];
	if ([n isEqualToString:@"context-menu"])
		return [NSCursor contextualMenuCursor];
	if ([n isEqualToString:@"help"])
		return [NSCursor arrowCursor]; // no help cursor in public API
	if ([n isEqualToString:@"wait"] || [n isEqualToString:@"busy"] || [n isEqualToString:@"progress"])
		return [NSCursor arrowCursor]; // no wait cursor in public API
	if ([n isEqualToString:@"zoom-in"] || [n isEqualToString:@"zoom-out"])
		return [NSCursor arrowCursor]; // no zoom cursor in public API

	// 'default', 'auto', 'none', 'arrow', or anything unrecognised —
	// let WKWebView handle it normally.
	return nil;
}

static void setCursorByName(const char* name) {
	// Copy the C string into an NSString BEFORE dispatching — the Go caller
	// frees the C string (via defer C.free) before the async block executes,
	// so capturing the raw pointer would be a use-after-free.
	NSString *n = [NSString stringWithUTF8String:name];
	dispatch_async(dispatch_get_main_queue(), ^{
		NSCursor *cursor = cursorForName(n);

		if (cursor) {
			_desiredCursor = cursor;
			[cursor set];

			// Install a mouse-moved monitor to continuously enforce the cursor.
			// WKWebView's NSTrackingArea resets the cursor on every mouse move,
			// so we need to re-apply ours after each move event.
			if (!_cursorMonitor) {
				_cursorMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskMouseMoved
					handler:^NSEvent*(NSEvent *event) {
						if (_desiredCursor) {
							[_desiredCursor set];
						}
						return event;
					}];
			}
		} else {
			// Standard cursor — stop overriding and let WKWebView manage normally.
			_desiredCursor = nil;
			if (_cursorMonitor) {
				[NSEvent removeMonitor:_cursorMonitor];
				_cursorMonitor = nil;
			}
			[[NSCursor arrowCursor] set];
		}
	});
}
*/
import "C"

import "unsafe"

// SetCursor sets the native macOS cursor by CSS cursor name.
// Called from the frontend via the WKWebView cursor bridge.
// For non-default cursors, installs a mouse-moved event monitor to
// continuously enforce the cursor (WKWebView's tracking areas would
// otherwise reset it on every mouse move).
func (a *App) SetCursor(name string) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	C.setCursorByName(cName)
}
