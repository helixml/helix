package tools

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store/sqlite"
)

// TestRenderEvent covers the parsed-line → transcript-body rules. Each
// claude stream-json line maps to zero or more bodies, one per atomic
// segment (assistant text, tool_use, tool_result, system init, run
// result). Non-renderable types (e.g. unknown subtypes) yield nothing.
func TestRenderEvent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ev   streamEvent
		want []string
	}{
		{
			name: "system init",
			ev:   streamEvent{Type: "system", Subtype: "init"},
			want: []string{"--- session start ---"},
		},
		{
			name: "system other subtype is ignored",
			ev:   streamEvent{Type: "system", Subtype: "compact"},
			want: nil,
		},
		{
			name: "result success",
			ev:   streamEvent{Type: "result", Result: "all done"},
			want: []string{"result: all done"},
		},
		{
			name: "result error",
			ev:   streamEvent{Type: "result", Result: "boom", IsError: true},
			want: []string{"result-error: boom"},
		},
		{
			name: "assistant text + tool_use as separate bodies",
			ev: streamEvent{
				Type: "assistant",
				Message: jsonRaw(`{"role":"assistant","content":[
					{"type":"text","text":"hi there"},
					{"type":"tool_use","name":"publish","input":{"streamId":"s-x","body":"y"}}
				]}`),
			},
			want: []string{
				"assistant: hi there",
				`tool_use publish: {"streamId":"s-x","body":"y"}`,
			},
		},
		{
			name: "assistant empty text segment is skipped",
			ev: streamEvent{
				Type: "assistant",
				Message: jsonRaw(`{"role":"assistant","content":[
					{"type":"text","text":""}
				]}`),
			},
			want: nil,
		},
		{
			// tool_result.content is rendered as JSON — claude can return
			// either a bare string or a structured object, so we never
			// strip the quotes.
			name: "user tool_result success",
			ev: streamEvent{
				Type: "user",
				Message: jsonRaw(`{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"t1","content":"ok"}
				]}`),
			},
			want: []string{`tool_result: "ok"`},
		},
		{
			name: "user tool_result error",
			ev: streamEvent{
				Type: "user",
				Message: jsonRaw(`{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"t1","content":"nope","is_error":true}
				]}`),
			},
			want: []string{`tool_result-error: "nope"`},
		},
		{
			name: "non-tool_result user segments are ignored",
			ev: streamEvent{
				Type:    "user",
				Message: jsonRaw(`{"role":"user","content":[{"type":"text","text":"x"}]}`),
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := renderEvent(tc.ev)
			if !equalSlice(got, tc.want) {
				t.Fatalf("renderEvent = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStreamTranscriptPublishesPerSegment verifies the parser walks
// claude's stream-json output and emits one publish call per atomic
// segment. Non-JSON lines are passed through verbatim.
func TestStreamTranscriptPublishesPerSegment(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"publish","input":{"x":1}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`not-json-but-should-still-show-up`,
		`{"type":"result","result":"done"}`,
	}, "\n")

	var got []string
	streamTranscript(strings.NewReader(input), func(body string) {
		got = append(got, body)
	})

	want := []string{
		"--- session start ---",
		"assistant: hello",
		`tool_use publish: {"x":1}`,
		`tool_result: "ok"`,
		"not-json-but-should-still-show-up",
		"result: done",
	}
	if !equalSlice(got, want) {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

// TestPublishActivationEventAppendsAndNotifies wires a real SQLite store
// and broadcaster, then exercises publishActivationEvent end to end:
// the event must land on the activation stream, attributed to the
// Worker, and any long-poll observer subscribed to that stream must
// wake.
func TestPublishActivationEventAppendsAndNotifies(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	// The activation stream needs to exist before events can be appended
	// (Append is permissive but ListForStream is what we verify against).
	streamID := activationStreamID("w-x")
	stream, err := domain.NewStream(streamID, "Activations: w-x", "test", "w-owner", now, domain.Transport{})
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := s.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	bc := broadcast.New()
	wake := bc.Subscribe([]domain.StreamID{streamID})
	t.Cleanup(func() { bc.Unsubscribe([]domain.StreamID{streamID}, wake) })

	cfg := ClaudeSpawnerConfig{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:       s,
		Broadcaster: bc,
		Now:         func() time.Time { return now },
		NewID:       func() string { return "id-1" },
	}

	publishActivationEvent(ctx, cfg, "w-x", streamID, "assistant: hello")

	events, err := s.Events.ListForStream(ctx, streamID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one", events)
	}
	if events[0].Source != "w-x" {
		t.Fatalf("source = %q, want w-x", events[0].Source)
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if msg.Body != "assistant: hello" {
		t.Fatalf("message body = %q", msg.Body)
	}
	if msg.From != "w-x" {
		t.Fatalf("message from = %q, want w-x", msg.From)
	}

	select {
	case <-wake:
	default:
		t.Fatalf("broadcaster did not wake long-poll observer")
	}

	// Empty body is a no-op (would fail domain.NewEvent validation).
	publishActivationEvent(ctx, cfg, "w-x", streamID, "")
	events, _ = s.Events.ListForStream(ctx, streamID, 10)
	if len(events) != 1 {
		t.Fatalf("empty body should not append; events = %d", len(events))
	}
}

func jsonRaw(s string) []byte { return []byte(s) }

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
