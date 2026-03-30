package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func TestOutboxEnqueueAndPending(t *testing.T) {
	ob := NewOutbox()

	req := &types.SessionChatRequest{
		SessionID: "ses_1",
		Messages: []*types.Message{
			{Role: "user", Content: types.MessageContent{Parts: []any{"hello"}}},
		},
	}

	id := ob.Enqueue(req)
	if id == "" {
		t.Error("expected non-empty ID")
	}
	if ob.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", ob.PendingCount())
	}

	entry := ob.NextPending()
	if entry == nil {
		t.Fatal("expected pending entry")
	}
	if entry.ID != id {
		t.Error("expected same ID")
	}
}

func TestOutboxSendFlow(t *testing.T) {
	ob := NewOutbox()
	req := &types.SessionChatRequest{SessionID: "ses_1"}

	id := ob.Enqueue(req)

	ob.MarkSending(id)
	if ob.PendingCount() != 1 { // sending counts as pending
		t.Errorf("expected 1 pending during send, got %d", ob.PendingCount())
	}

	ob.MarkSent(id)
	if ob.PendingCount() != 0 {
		t.Errorf("expected 0 pending after sent, got %d", ob.PendingCount())
	}
}

func TestOutboxRetry(t *testing.T) {
	ob := NewOutbox()
	req := &types.SessionChatRequest{SessionID: "ses_1"}

	id := ob.Enqueue(req)

	// First two failures should retry
	ob.MarkSending(id)
	ob.MarkFailed(id, fmt.Errorf("network error"))
	if ob.PendingCount() != 1 {
		t.Error("expected retry after first failure")
	}

	ob.MarkSending(id)
	ob.MarkFailed(id, fmt.Errorf("network error"))
	if ob.PendingCount() != 1 {
		t.Error("expected retry after second failure")
	}

	// Third failure should give up
	ob.MarkSending(id)
	ob.MarkFailed(id, fmt.Errorf("network error"))
	if ob.PendingCount() != 0 {
		t.Error("expected failed (no more retries) after third failure")
	}
}

func TestOutboxCleanup(t *testing.T) {
	ob := NewOutbox()
	req := &types.SessionChatRequest{SessionID: "ses_1"}

	id := ob.Enqueue(req)
	ob.MarkSending(id)
	ob.MarkSent(id)

	// Should not clean up recent entries
	ob.Cleanup(time.Hour)
	// entry still exists (just sent)

	// Enqueue another
	id2 := ob.Enqueue(req)
	if id2 == id {
		t.Error("expected unique IDs")
	}
}

func TestOutboxOrdering(t *testing.T) {
	ob := NewOutbox()

	id1 := ob.Enqueue(&types.SessionChatRequest{SessionID: "ses_1"})
	id2 := ob.Enqueue(&types.SessionChatRequest{SessionID: "ses_2"})

	// Should return first entry
	entry := ob.NextPending()
	if entry.ID != id1 {
		t.Error("expected first enqueued entry first")
	}

	ob.MarkSending(id1)
	ob.MarkSent(id1)

	entry = ob.NextPending()
	if entry.ID != id2 {
		t.Error("expected second entry after first is sent")
	}
}
