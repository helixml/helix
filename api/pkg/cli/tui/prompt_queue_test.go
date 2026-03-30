package tui

import (
	"strings"
	"testing"
)

func TestPromptQueue_AddAndOrder(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "First (queue)", false)
	pq.Add("p2", "Second (queue)", false)
	pq.Add("p3", "Urgent (interrupt)", true)

	// Interrupt should be first
	if pq.Count() != 3 {
		t.Fatalf("expected 3, got %d", pq.Count())
	}

	next := pq.NextPending()
	if next.Content != "Urgent (interrupt)" {
		t.Errorf("expected interrupt first, got %q", next.Content)
	}
}

func TestPromptQueue_InterruptOrdering(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "queue-1", false)
	pq.Add("p2", "interrupt-1", true)
	pq.Add("p3", "queue-2", false)
	pq.Add("p4", "interrupt-2", true)

	// Should be: interrupt-1, interrupt-2, queue-1, queue-2
	pq.mu.Lock()
	if pq.prompts[0].Content != "interrupt-1" {
		t.Errorf("pos 0 should be interrupt-1, got %q", pq.prompts[0].Content)
	}
	if pq.prompts[1].Content != "interrupt-2" {
		t.Errorf("pos 1 should be interrupt-2, got %q", pq.prompts[1].Content)
	}
	if pq.prompts[2].Content != "queue-1" {
		t.Errorf("pos 2 should be queue-1, got %q", pq.prompts[2].Content)
	}
	if pq.prompts[3].Content != "queue-2" {
		t.Errorf("pos 3 should be queue-2, got %q", pq.prompts[3].Content)
	}
	pq.mu.Unlock()
}

func TestPromptQueue_ToggleInterrupt(t *testing.T) {
	pq := NewPromptQueue()

	// Add: queue first, then interrupt. After Add, order is [interrupt, queue]
	pq.Add("p1", "was-queue", false)
	pq.Add("p2", "was-interrupt", true)
	// State: [was-interrupt(0), was-queue(1)]

	// Toggle index 0 (was-interrupt) OFF → becomes queue, moves to end
	pq.ToggleInterrupt(0)
	// State: [was-queue(0), was-interrupt(1)] — was-interrupt is now non-interrupt at end

	pq.mu.Lock()
	if pq.prompts[0].Content != "was-queue" {
		t.Errorf("pos 0 should be was-queue, got %q", pq.prompts[0].Content)
	}
	if pq.prompts[1].Content != "was-interrupt" {
		t.Errorf("pos 1 should be was-interrupt, got %q", pq.prompts[1].Content)
	}
	if pq.prompts[1].Interrupt {
		t.Error("was-interrupt should now be non-interrupt after toggle")
	}
	pq.mu.Unlock()
}

func TestPromptQueue_EditPause(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "original text", false)

	if pq.IsPaused() {
		t.Error("should not be paused initially")
	}

	pq.StartEdit(0)
	if !pq.IsPaused() {
		t.Error("should be paused during edit")
	}

	next := pq.NextPending()
	if next != nil {
		t.Error("should not return pending while paused")
	}

	pq.FinishEdit(0, "edited text")
	if pq.IsPaused() {
		t.Error("should not be paused after finish edit")
	}

	pq.mu.Lock()
	if pq.prompts[0].Content != "edited text" {
		t.Errorf("expected edited text, got %q", pq.prompts[0].Content)
	}
	pq.mu.Unlock()
}

func TestPromptQueue_CancelEdit(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "original", false)
	pq.StartEdit(0)
	pq.CancelEdit()

	if pq.IsPaused() {
		t.Error("should not be paused after cancel")
	}

	pq.mu.Lock()
	if pq.prompts[0].Content != "original" {
		t.Error("content should be unchanged after cancel")
	}
	pq.mu.Unlock()
}

func TestPromptQueue_Remove(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "first", false)
	pq.Add("p2", "second", false)
	pq.Remove(0)

	if pq.Count() != 1 {
		t.Errorf("expected 1 after remove, got %d", pq.Count())
	}

	pq.mu.Lock()
	if pq.prompts[0].Content != "second" {
		t.Errorf("expected 'second', got %q", pq.prompts[0].Content)
	}
	pq.mu.Unlock()
}

func TestPromptQueue_MarkSent(t *testing.T) {
	pq := NewPromptQueue()

	pq.Add("p1", "sent", false)
	pq.Add("p2", "kept", false)
	pq.MarkSent("p1")

	if pq.Count() != 1 {
		t.Errorf("expected 1 after mark sent, got %d", pq.Count())
	}
}

func TestPromptQueue_View(t *testing.T) {
	pq := NewPromptQueue()
	pq.SetWidth(80)

	if pq.View() != "" {
		t.Error("empty queue should render nothing")
	}

	pq.Add("p1", "Fix the bug", false)
	pq.Add("p2", "Urgent fix", true)

	view := pq.View()
	if !strings.Contains(view, "Queued prompts (2)") {
		t.Error("should show count in header")
	}
	if !strings.Contains(view, "[interrupt]") {
		t.Error("should show interrupt badge")
	}
	if !strings.Contains(view, "Fix the bug") {
		t.Error("should show prompt content")
	}
}
