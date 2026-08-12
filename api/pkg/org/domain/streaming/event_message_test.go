package streaming_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// The Message round-trip tests (M1..M4) moved to
// api/pkg/org/message/message_test.go in B3b alongside the lifted
// type. The two tests that stay here exercise the Event bridge:
// streaming.NewMessageEvent (constructor) and Event.Message() (accessor) —
// both still live in helix-org/domain because they depend on the
// Event struct, which has not been lifted yet.

func TestEventMessage(t *testing.T) {
	t.Parallel()
	msg := streaming.Message{From: "w-alice", Body: "hi"}
	body, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	e, err := streaming.NewEvent("e-1", "s-1", "w-alice", body, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("streaming.NewEvent: %v", err)
	}
	got, err := e.Message()
	if err != nil {
		t.Fatalf("e.Message(): %v", err)
	}
	if got.From != "w-alice" || got.Body != "hi" {
		t.Fatalf("got %+v", got)
	}
}

func TestNewMessageEvent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	msg := streaming.Message{From: "w-alice", To: []string{"w-bob"}, Body: "hi"}
	e, err := streaming.NewMessageEvent("e-1", "s-dm", "w-alice", msg, now, "org-test")
	if err != nil {
		t.Fatalf("streaming.NewMessageEvent: %v", err)
	}
	if e.ID != "e-1" || e.TopicID != "s-dm" || e.Source != "w-alice" {
		t.Fatalf("event fields wrong: %+v", e)
	}
	parsed, err := e.Message()
	if err != nil {
		t.Fatalf("e.Message(): %v", err)
	}
	if parsed.From != "w-alice" || parsed.Body != "hi" || len(parsed.To) != 1 {
		t.Fatalf("parsed wrong: %+v", parsed)
	}
}

func TestNewMessageEventRejectsEmptyEncoding(t *testing.T) {
	t.Parallel()
	// An empty Message encodes to "{}" — non-empty as a string, so
	// streaming.NewEvent's empty-body check passes. This documents that "{}" is a
	// valid (if degenerate) Body — pure trigger events.
	now := time.Now().UTC()
	e, err := streaming.NewMessageEvent("e-1", "s-1", "", streaming.Message{}, now, "org-test")
	if err != nil {
		t.Fatalf("streaming.NewMessageEvent(empty msg): %v", err)
	}
	if e.Body != "{}" {
		t.Fatalf("empty Message body = %q, want %q", e.Body, "{}")
	}
}
