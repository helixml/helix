package tui

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationManager_Basic(t *testing.T) {
	nm := NewNotificationManager()

	if nm.UnreadCount() != 0 {
		t.Error("expected 0 unread initially")
	}

	nm.Add(&Notification{
		ID:        "n1",
		Type:      NotifySpecReady,
		Title:     "Specs ready for review",
		TaskID:    "spt_1",
		Timestamp: time.Now(),
	})

	if nm.UnreadCount() != 1 {
		t.Errorf("expected 1 unread, got %d", nm.UnreadCount())
	}

	if nm.UnreadForTask("spt_1") != 1 {
		t.Error("expected 1 unread for spt_1")
	}
	if nm.UnreadForTask("spt_2") != 0 {
		t.Error("expected 0 unread for spt_2")
	}
}

func TestNotificationManager_MarkRead(t *testing.T) {
	nm := NewNotificationManager()

	nm.Add(&Notification{ID: "n1", TaskID: "spt_1", Timestamp: time.Now()})
	nm.Add(&Notification{ID: "n2", TaskID: "spt_2", Timestamp: time.Now()})

	nm.MarkTaskRead("spt_1")
	if nm.UnreadCount() != 1 {
		t.Errorf("expected 1 unread after marking spt_1, got %d", nm.UnreadCount())
	}

	nm.MarkAllRead()
	if nm.UnreadCount() != 0 {
		t.Error("expected 0 unread after mark all read")
	}
}

func TestNotificationManager_Toggle(t *testing.T) {
	nm := NewNotificationManager()

	if nm.IsVisible() {
		t.Error("should not be visible initially")
	}

	nm.Toggle()
	if !nm.IsVisible() {
		t.Error("should be visible after toggle")
	}

	nm.Toggle()
	if nm.IsVisible() {
		t.Error("should not be visible after second toggle")
	}
}

func TestNotificationManager_Render(t *testing.T) {
	nm := NewNotificationManager()
	nm.Add(&Notification{
		ID:        "n1",
		Type:      NotifyPRCreated,
		Title:     "PR created for Fix login",
		TaskID:    "spt_1",
		Timestamp: time.Now().Add(-5 * time.Minute),
	})
	nm.Add(&Notification{
		ID:        "n2",
		Type:      NotifyAgentError,
		Title:     "Agent error on Refactor DB",
		TaskID:    "spt_3",
		Timestamp: time.Now().Add(-1 * time.Hour),
	})

	out := nm.Render(120, 30)
	if !strings.Contains(out, "Notifications") {
		t.Error("expected 'Notifications' header")
	}
	if !strings.Contains(out, "2 unread") {
		t.Error("expected '2 unread' count")
	}
	if !strings.Contains(out, "PR created") {
		t.Error("expected notification title")
	}
}
