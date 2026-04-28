package tools

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store/sqlite"
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

// TestRenderTriggerGitHub: a github-shaped event (issue.opened) must
// surface every populated envelope field in the prompt, including
// Subject (issue title), From (sender.login), ThreadID (#42), and
// Extra (the full webhook body with the synthetic `event` key
// injected). Without this, the Worker only sees Body and has to
// call read_events to learn what kind of trigger fired — which is
// exactly the bug that caused the docs-engineer to misroute issue
// #3 to PR #2 during the README's E2E run.
func TestRenderTriggerGitHub(t *testing.T) {
	t.Parallel()

	extra := []byte(`{"action":"opened","event":"issues","issue":{"id":12345,"number":42,"title":"x","body":"y"},"sender":{"login":"philwinder"},"repository":{"full_name":"helixml/helix-org"}}`)
	tr := Trigger{
		Kind:      TriggerEvent,
		EventID:   "e-abc",
		StreamID:  "s-github",
		Source:    "", // system-emitted (inbound webhook)
		CreatedAt: time.Date(2026, 4, 28, 12, 27, 23, 0, time.UTC),
		Message: domain.Message{
			From:      "philwinder",
			Subject:   "README setup steps mention an env var that no longer exists",
			Body:      "Step 3 references HELIX_FOO; the code reads HELIX_BAR now.",
			ThreadID:  "#42",
			MessageID: "delivery-uuid-1",
			Extra:     extra,
		},
	}

	got := renderTrigger(tr)

	wants := []string{
		"stream:      s-github",
		"event:       e-abc",
		"time:        2026-04-28T12:27:23Z",
		"from:        philwinder",
		"subject:     README setup steps mention an env var that no longer exists",
		"thread_id:   #42",
		"message_id:  delivery-uuid-1",
		"Step 3 references HELIX_FOO",                    // body content
		`"event":"issues"`,                               // Extra includes the synthetic event key
		`"action":"opened"`,                              // Extra preserves the upstream action
		`"sender":{"login":"philwinder"}`,                // Extra preserves nested objects
		`"repository":{"full_name":"helixml/helix-org"}`, // Extra preserves repo info
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}

	// Empty fields must be omitted (cleanliness), not rendered as
	// "to: ", "in_reply_to: ", etc.
	for _, omit := range []string{"to:", "in_reply_to:", "source:"} {
		if strings.Contains(got, omit) {
			t.Errorf("renderTrigger output should omit empty %q\n--- output ---\n%s", omit, got)
		}
	}
}

// TestRenderTriggerEmail: an email-shaped event must surface
// To, InReplyTo, ThreadID — which is how the email demo's customer-
// service role pairs replies back to the original thread. The
// previous prompt format only carried Body, forcing the role to
// call read_events for the headers; this test pins the fix.
func TestRenderTriggerEmail(t *testing.T) {
	t.Parallel()

	tr := Trigger{
		Kind:      TriggerEvent,
		EventID:   "e-1",
		StreamID:  "s-support",
		Source:    "", // inbound
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		Message: domain.Message{
			From:      "alice@example.com",
			To:        []string{"abc123+sam@inbound.postmarkapp.com"},
			Subject:   "[eng] Re: Webhook stream isn't firing",
			Body:      "Most webhook flow issues are config or subscription mismatches.",
			ThreadID:  "<root@example.com>",
			InReplyTo: "<original@example.com>",
			MessageID: "<msg-2@example.com>",
		},
	}

	got := renderTrigger(tr)

	wants := []string{
		"from:        alice@example.com",
		"to:          abc123+sam@inbound.postmarkapp.com",
		"subject:     [eng] Re: Webhook stream isn't firing",
		"thread_id:   <root@example.com>",
		"in_reply_to: <original@example.com>",
		"message_id:  <msg-2@example.com>",
		"Most webhook flow issues",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}
}

// TestRenderTriggerWorkerPublished: when an internal Worker
// publishes a plain message (no Subject, no ThreadID, just Body),
// the rendered prompt should still carry source (the publisher's
// WorkerID) and from, but skip every empty field.
func TestRenderTriggerWorkerPublished(t *testing.T) {
	t.Parallel()

	tr := Trigger{
		Kind:      TriggerEvent,
		EventID:   "e-1",
		StreamID:  "s-general",
		Source:    "w-alice",
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		Message: domain.Message{
			From: "w-alice",
			Body: "hello",
		},
	}
	got := renderTrigger(tr)

	for _, w := range []string{"source:      w-alice", "from:        w-alice", "hello"} {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}
	for _, omit := range []string{"to:", "subject:", "thread_id:", "in_reply_to:", "message_id:", "extra:"} {
		if strings.Contains(got, omit) {
			t.Errorf("renderTrigger output should omit empty %q\n--- output ---\n%s", omit, got)
		}
	}
}

// TestBuildPromptIncludesEnvelope checks the integration: a Trigger
// with full envelope fields produces a prompt whose === Trigger ===
// section carries all of them. Guards against a future refactor
// that decouples renderTrigger from buildPrompt.
func TestBuildPromptIncludesEnvelope(t *testing.T) {
	t.Parallel()

	tr := Trigger{
		Kind:      TriggerEvent,
		EventID:   "e-abc",
		StreamID:  "s-github",
		CreatedAt: time.Date(2026, 4, 28, 12, 27, 23, 0, time.UTC),
		Message: domain.Message{
			From:    "philwinder",
			Subject: "Confusing example in the docs",
			Body:    "The README has an install command that doesn't run as written.",
			Extra:   []byte(`{"event":"issues","action":"opened"}`),
		},
	}
	prompt := buildPrompt("w-doc-engineer", "[role.md contents]", tr)

	if !strings.Contains(prompt, "=== Trigger ===") || !strings.Contains(prompt, "=== end trigger ===") {
		t.Fatalf("trigger fences missing\n%s", prompt)
	}
	for _, w := range []string{
		"subject:     Confusing example in the docs",
		"from:        philwinder",
		`"event":"issues"`,
	} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt missing %q", w)
		}
	}
}

// TestProjectEnvWritesCanonicalState pins the contract: projectEnv
// reads the worker / position / role from the store and writes
// role.md, identity.md, and agent.md into envPath. Subsequent
// activations re-run this so updates land before claude is exec'd —
// no separate fan-out tool needed.
func TestProjectEnvWritesCanonicalState(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	role, _ := domain.NewRole("r-eng", "# Role: Engineer\nBuild stuff.", now)
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	worker, _ := domain.NewAIWorker("w-eng", []domain.PositionID{"p-eng"}, "# Persona\nAlice.")
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
		"agent.md":    agentMDStub,
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

	// Update the role in the DB; re-project; the new content lands.
	updated := domain.Role{ID: role.ID, Content: "# Role: Engineer v2", CreatedAt: role.CreatedAt, UpdatedAt: now}
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

	// Same for identity via the domain.
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
