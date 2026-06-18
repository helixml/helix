package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// setClipboardImagePNG writes raw PNG bytes to the general pasteboard
// under NSPasteboardTypePNG. Returns 0 on success, 1 on failure.
static int setClipboardImagePNG(const void* bytes, int length) {
	if (bytes == NULL || length <= 0) return 1;
	NSData *data = [NSData dataWithBytes:bytes length:length];
	if (data == nil) return 1;
	NSPasteboard *pb = [NSPasteboard generalPasteboard];
	[pb clearContents];
	BOOL ok = [pb setData:data forType:NSPasteboardTypePNG];
	return ok ? 0 : 1;
}

// getClipboardImagePNGLength returns the length of any image/png payload
// currently on the general pasteboard, or 0 if none. Callers should then
// allocate a buffer of that size and call getClipboardImagePNGBytes.
static int getClipboardImagePNGLength(void) {
	NSPasteboard *pb = [NSPasteboard generalPasteboard];
	NSData *data = [pb dataForType:NSPasteboardTypePNG];
	if (data == nil) {
		// Fall back to TIFF — many macOS apps put images as TIFF, we convert.
		NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
		if (tiff == nil) return 0;
		NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
		if (rep == nil) return 0;
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (png == nil) return 0;
		return (int)[png length];
	}
	return (int)[data length];
}

// getClipboardImagePNGBytes copies PNG bytes into the caller-provided buffer.
// Returns the number of bytes copied. Re-fetches from the pasteboard rather
// than caching, which keeps the call self-contained at the cost of one extra
// pasteboard read (cheap).
static int getClipboardImagePNGBytes(void* buf, int bufLen) {
	NSPasteboard *pb = [NSPasteboard generalPasteboard];
	NSData *data = [pb dataForType:NSPasteboardTypePNG];
	if (data == nil) {
		NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
		if (tiff == nil) return 0;
		NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
		if (rep == nil) return 0;
		data = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (data == nil) return 0;
	}
	int n = (int)[data length];
	if (n > bufLen) n = bufLen;
	memcpy(buf, [data bytes], n);
	return n;
}
*/
import "C"

import (
	"encoding/base64"
	"fmt"
	"unsafe"
)

// SetClipboardImagePNG accepts a base64-encoded PNG and writes it to the
// macOS general pasteboard as NSPasteboardTypePNG. Used by the desktop
// stream iframe via postMessage to round-trip image clipboard data
// (WKWebView blocks navigator.clipboard in iframes).
func (a *App) SetClipboardImagePNG(base64PNG string) error {
	if base64PNG == "" {
		return fmt.Errorf("empty base64 payload")
	}
	bytes, err := base64.StdEncoding.DecodeString(base64PNG)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	if len(bytes) == 0 {
		return fmt.Errorf("decoded payload is empty")
	}
	rc := C.setClipboardImagePNG(unsafe.Pointer(&bytes[0]), C.int(len(bytes)))
	if rc != 0 {
		return fmt.Errorf("NSPasteboard setData failed")
	}
	return nil
}

// GetClipboardImagePNG returns the current general-pasteboard image as
// base64-encoded PNG bytes, or "" if no image is on the pasteboard.
// Accepts both native PNG and TIFF (the latter transcoded to PNG, since
// screenshots taken via Cmd+Shift+Ctrl+4 are placed as TIFF).
func (a *App) GetClipboardImagePNG() (string, error) {
	n := int(C.getClipboardImagePNGLength())
	if n <= 0 {
		return "", nil
	}
	buf := make([]byte, n)
	got := int(C.getClipboardImagePNGBytes(unsafe.Pointer(&buf[0]), C.int(n)))
	if got <= 0 {
		return "", nil
	}
	return base64.StdEncoding.EncodeToString(buf[:got]), nil
}
