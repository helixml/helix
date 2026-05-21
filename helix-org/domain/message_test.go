package domain

import (
	"strings"
	"testing"
	"time"
)

func TestMessageRoundTrip(t *testing.T) {
	t.Parallel()
	msg := Message{
		From:            "w-alice",
		To:              []string{"w-bob"},
		Subject:         "hi",
		Body:            "hello\nthere",
		BodyContentType: "text/plain",
		ThreadID:        "t-123",
		InReplyTo:       "m-prev",
		MessageID:       "m-now",
		Attachments: []Attachment{
			{Filename: "x.pdf", ContentType: "application/pdf", URL: "https://e.com/x", SizeBytes: 1024},
		},
	}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if got.From != msg.From || got.Body != msg.Body || len(got.To) != 1 || got.To[0] != "w-bob" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.ThreadID != msg.ThreadID || got.InReplyTo != msg.InReplyTo || got.MessageID != msg.MessageID {
		t.Fatalf("threading mismatch: %+v", got)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "x.pdf" {
		t.Fatalf("attachment lost: %+v", got.Attachments)
	}
}

func TestMessageMinimal(t *testing.T) {
	t.Parallel()
	// Only Body set is valid — most internal events look like this.
	msg := Message{Body: "hello"}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(encoded, `"body":"hello"`) {
		t.Fatalf("expected body field in JSON, got %q", encoded)
	}
	if strings.Contains(encoded, `"from"`) || strings.Contains(encoded, `"to"`) {
		t.Fatalf("unset fields should be omitted, got %q", encoded)
	}
}

func TestMessageEmpty(t *testing.T) {
	t.Parallel()
	// Empty Message — pure trigger pulse — is also valid; encodes to "{}".
	msg := Message{}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if encoded != "{}" {
		t.Fatalf("empty message = %q, want %q", encoded, "{}")
	}
}

func TestDecodeMessageMalformed(t *testing.T) {
	t.Parallel()
	cases := []string{
		``,
		`not json`,
		`{`,
		`[`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeMessage(c); err == nil {
				t.Fatalf("DecodeMessage(%q) = nil, want error", c)
			}
		})
	}
}

func TestEventMessage(t *testing.T) {
	t.Parallel()
	msg := Message{From: "w-alice", Body: "hi"}
	body, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	e, err := NewEvent("e-1", "s-1", "w-alice", body, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
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
	msg := Message{From: "w-alice", To: []string{"w-bob"}, Body: "hi"}
	e, err := NewMessageEvent("e-1", "s-dm", "w-alice", msg, now)
	if err != nil {
		t.Fatalf("NewMessageEvent: %v", err)
	}
	if e.ID != "e-1" || e.StreamID != "s-dm" || e.Source != "w-alice" {
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
	// NewEvent's empty-body check passes. This documents that "{}" is a
	// valid (if degenerate) Body — pure trigger events.
	now := time.Now().UTC()
	e, err := NewMessageEvent("e-1", "s-1", "", Message{}, now)
	if err != nil {
		t.Fatalf("NewMessageEvent(empty msg): %v", err)
	}
	if e.Body != "{}" {
		t.Fatalf("empty Message body = %q, want %q", e.Body, "{}")
	}
}
