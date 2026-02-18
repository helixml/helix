package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications

#import <UserNotifications/UserNotifications.h>

// HelixNotificationDelegate allows notifications to display even when
// the app is in the foreground (macOS suppresses them by default).
@interface HelixNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation HelixNotificationDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
	willPresentNotification:(UNNotification *)notification
	withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
	// Show banner + play sound even when Helix is the active app
	completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
}
@end

static HelixNotificationDelegate *_notifDelegate = nil;

// initNotificationAuth requests notification authorization from macOS
// and installs the foreground presentation delegate.
// Dispatches to the main thread — UNUserNotificationCenter requires it.
static void initNotificationAuth() {
	dispatch_async(dispatch_get_main_queue(), ^{
		UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

		// Install delegate so notifications show while app is in foreground
		if (!_notifDelegate) {
			_notifDelegate = [[HelixNotificationDelegate alloc] init];
			center.delegate = _notifDelegate;
		}

		[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
			completionHandler:^(BOOL granted, NSError *error) {
				if (error) {
					NSLog(@"[Helix] Notification auth error: %@", error);
				} else {
					NSLog(@"[Helix] Notification auth granted: %d", granted);
				}
			}];
	});
}

// postNotification sends a native macOS notification via UNUserNotificationCenter.
// Dispatches to the main thread for safety.
static void postNotification(const char *title, const char *body) {
	// Copy strings before the block captures them (they're freed by Go after this returns)
	NSString *nsTitle = [NSString stringWithUTF8String:title];
	NSString *nsBody = [NSString stringWithUTF8String:body];

	dispatch_async(dispatch_get_main_queue(), ^{
		UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
		content.title = nsTitle;
		content.body = nsBody;
		content.sound = [UNNotificationSound defaultSound];

		NSString *identifier = [[NSUUID UUID] UUIDString];
		UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
			content:content trigger:nil];

		UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
		[center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
			if (error) {
				NSLog(@"[Helix] Failed to post notification: %@", error);
			} else {
				NSLog(@"[Helix] Notification posted: %@", nsTitle);
			}
		}];
	});
}
*/
import "C"

import (
	"log"
	"unsafe"
)

// initNotifications requests notification permission from macOS and installs
// the delegate that allows foreground notifications. Call once during app startup.
// Dispatches to the main thread internally.
func initNotifications() {
	C.initNotificationAuth()
	log.Println("Requested macOS notification authorization")
}

// sendNotification posts a native macOS notification.
// Non-blocking; dispatches to the main thread internally.
func sendNotification(title, message string) {
	cTitle := C.CString(title)
	cBody := C.CString(message)
	C.postNotification(cTitle, cBody)
	C.free(unsafe.Pointer(cTitle))
	C.free(unsafe.Pointer(cBody))
}
