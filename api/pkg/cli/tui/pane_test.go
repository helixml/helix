package tui

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func newTestChat(taskID string) *ChatModel {
	return &ChatModel{
		task: &types.SpecTask{ID: taskID, Name: "Test " + taskID},
	}
}

func TestPaneManagerOpen(t *testing.T) {
	pm := NewPaneManager()
	pm.SetSize(120, 40)

	if !pm.IsEmpty() {
		t.Error("expected empty pane manager")
	}

	chat := newTestChat("t1")
	pm.OpenPane(chat)

	if pm.IsEmpty() {
		t.Error("expected non-empty after open")
	}
	if pm.PaneCount() != 1 {
		t.Errorf("expected 1 pane, got %d", pm.PaneCount())
	}
	if pm.FocusedChat() != chat {
		t.Error("focused chat should be the opened one")
	}
}

func TestPaneManagerSplit(t *testing.T) {
	pm := NewPaneManager()
	pm.SetSize(120, 40)

	chat1 := newTestChat("t1")
	chat2 := newTestChat("t2")

	pm.OpenPane(chat1)
	pm.SplitFocused(SplitVertical, chat2)

	if pm.PaneCount() != 2 {
		t.Errorf("expected 2 panes, got %d", pm.PaneCount())
	}
	if pm.FocusedChat() != chat2 {
		t.Error("focused should be the new pane after split")
	}
}

func TestPaneManagerFocusCycle(t *testing.T) {
	pm := NewPaneManager()
	pm.SetSize(120, 40)

	chat1 := newTestChat("t1")
	chat2 := newTestChat("t2")
	chat3 := newTestChat("t3")

	pm.OpenPane(chat1)
	pm.SplitFocused(SplitVertical, chat2)
	pm.SplitFocused(SplitHorizontal, chat3)

	if pm.PaneCount() != 3 {
		t.Errorf("expected 3 panes, got %d", pm.PaneCount())
	}

	// Focus should be on chat3
	if pm.FocusedChat() != chat3 {
		t.Error("expected chat3 focused")
	}

	// Cycle forward
	pm.FocusNext()
	if pm.FocusedChat() == chat3 {
		t.Error("expected focus to move after FocusNext")
	}

	// Cycle back
	pm.FocusPrev()
	if pm.FocusedChat() != chat3 {
		t.Error("expected focus back on chat3")
	}
}

func TestPaneManagerClose(t *testing.T) {
	pm := NewPaneManager()
	pm.SetSize(120, 40)

	chat1 := newTestChat("t1")
	chat2 := newTestChat("t2")

	pm.OpenPane(chat1)
	pm.SplitFocused(SplitVertical, chat2)

	// Close focused (chat2)
	ok := pm.CloseFocused()
	if !ok {
		t.Error("expected panes remaining after close")
	}
	if pm.PaneCount() != 1 {
		t.Errorf("expected 1 pane after close, got %d", pm.PaneCount())
	}
	if pm.FocusedChat() != chat1 {
		t.Error("expected chat1 focused after closing chat2")
	}

	// Close last pane
	ok = pm.CloseFocused()
	if ok {
		t.Error("expected no panes remaining")
	}
	if !pm.IsEmpty() {
		t.Error("expected empty after closing all")
	}
}

func TestPaneManagerRender(t *testing.T) {
	pm := NewPaneManager()
	pm.SetSize(80, 24)

	chat := newTestChat("t1")
	chat.SetSize(80, 24)
	pm.OpenPane(chat)

	output := pm.Render()
	if output == "" {
		t.Error("expected non-empty render")
	}
}
