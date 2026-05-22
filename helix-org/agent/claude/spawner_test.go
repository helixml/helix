package claude

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store/sqlite"
)

// readFile reads a single file inside dir. Tests use t.TempDir() so
// path traversal isn't a concern.
func readFile(dir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // dir is t.TempDir()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TestRenderEvent covers the parsed-line → transcript-body rules.
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

func TestPublishActivationEventAppendsAndNotifies(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	streamID := agent.ActivationStreamID("w-x")
	str, err := domain.NewStream(streamID, "Activations: w-x", "test", "w-owner", now, transport.Transport{})
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := s.Streams.Create(ctx, str); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	bc := broadcast.New()
	wake := bc.Subscribe([]stream.ID{streamID})
	t.Cleanup(func() { bc.Unsubscribe([]stream.ID{streamID}, wake) })

	cfg := SpawnerConfig{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:       s,
		Hub: bc,
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

	publishActivationEvent(ctx, cfg, "w-x", streamID, "")
	events, _ = s.Events.ListForStream(ctx, streamID, 10)
	if len(events) != 1 {
		t.Fatalf("empty body should not append; events = %d", len(events))
	}
}

func TestProjectEnvWritesCanonicalState(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	r, _ := role.New("r-eng", "# Role: Engineer\nBuild stuff.", nil, nil, now)
	if err := s.Roles.Create(ctx, r); err != nil {
		t.Fatalf("create role: %v", err)
	}
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	worker, _ := domain.NewAIWorker("w-eng", "p-eng", "# Persona\nAlice.")
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	envPath := t.TempDir()
	if err := projectEnv(ctx, s, "w-eng", envPath); err != nil {
		t.Fatalf("projectEnv: %v", err)
	}

	want := map[string]string{
		"role.md":     "# Role: Engineer\nBuild stuff.",
		"identity.md": "# Persona\nAlice.",
		"agent.md":    agent.Policy,
	}
	for name, expected := range want {
		got, err := readFile(envPath, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if got != expected {
			t.Errorf("%s = %q, want %q", name, got, expected)
		}
	}

	updated := role.Role{ID: r.ID, Content: "# Role: Engineer v2", CreatedAt: r.CreatedAt, UpdatedAt: now}
	if err := s.Roles.Update(ctx, updated); err != nil {
		t.Fatalf("update role: %v", err)
	}
	if err := projectEnv(ctx, s, "w-eng", envPath); err != nil {
		t.Fatalf("re-project: %v", err)
	}
	got, _ := readFile(envPath, "role.md")
	if got != "# Role: Engineer v2" {
		t.Fatalf("post-update role.md = %q", got)
	}

	if err := s.Workers.Update(ctx, worker.WithIdentityContent("# Persona\nAlice (v2).")); err != nil {
		t.Fatalf("update worker: %v", err)
	}
	if err := projectEnv(ctx, s, "w-eng", envPath); err != nil {
		t.Fatalf("re-project after identity update: %v", err)
	}
	got, _ = readFile(envPath, "identity.md")
	if got != "# Persona\nAlice (v2)." {
		t.Fatalf("post-update identity.md = %q", got)
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
