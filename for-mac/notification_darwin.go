package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications

#import <UserNotifications/UserNotifications.h>

// requestNotificationAuth requests notification authorization from macOS.
// Must be called once at startup. Safe to call multiple times (Apple no-ops
// if already granted/denied).
static void requestNotificationAuth() {
	UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
	[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
		completionHandler:^(BOOL granted, NSError *error) {
			if (error) {
				NSLog(@"[Helix] Notification auth error: %@", error);
			}
		}];
}

// postNotification sends a native macOS notification via UNUserNotificationCenter.
static void postNotification(const char *title, const char *body) {
	UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
	content.title = [NSString stringWithUTF8String:title];
	content.body = [NSString stringWithUTF8String:body];
	content.sound = [UNNotificationSound defaultSound];

	// Unique ID per notification so they don't replace each other
	NSString *identifier = [[NSUUID UUID] UUIDString];
	UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
		content:content trigger:nil];

	UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
	[center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
		if (error) {
			NSLog(@"[Helix] Failed to post notification: %@", error);
		}
	}];
}
*/
import "C"

import (
	"log"
	"unsafe"
)

// initNotifications requests notification permission from macOS.
// Call once during app startup.
func initNotifications() {
	C.requestNotificationAuth()
	log.Println("Requested macOS notification authorization")
}

// sendNotification posts a native macOS notification.
// Non-blocking; errors are logged by the Obj-C completion handler.
func sendNotification(title, message string) {
	cTitle := C.CString(title)
	cBody := C.CString(message)
	C.postNotification(cTitle, cBody)
	C.free(unsafe.Pointer(cTitle))
	C.free(unsafe.Pointer(cBody))
}
