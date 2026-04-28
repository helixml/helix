package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store/sqlite"
)

// TestPublishRejectsGitHubStream: publishing to a github transport
// stream is rejected with an explanatory error rather than a silent
// no-op. GitHub streams are inbound-only; outbound action lives in
// the Worker's `gh`. See design/github-transport.md.
func TestPublishRejectsGitHubStream(t *testing.T) {
	t.Parallel()

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	// Seed a github-transport Stream and a caller Worker.
	cfg, _ := json.Marshal(map[string]any{
		"repo":   "helixml/helix-org",
		"events": []string{"issues"},
	})
	stream, err := domain.NewStream("s-github", "s-github", "", "w-owner",
		time.Now().UTC(),
		domain.Transport{Kind: domain.TransportGitHub, Config: cfg})
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	caller, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"}, "")

	deps := DefaultDeps(st)
	tool := &Publish{deps: deps}

	args, _ := json.Marshal(map[string]any{
		"streamId": "s-github",
		"body":     "this should be rejected",
	})

	_, err = tool.Invoke(ctx, domain.Invocation{Caller: caller, Args: args})
	if err == nil {
		t.Fatalf("Invoke = nil, want error rejecting github publish")
	}
	if !strings.Contains(err.Error(), "github") {
		t.Fatalf("err = %v, want error mentioning github", err)
	}
	if !strings.Contains(err.Error(), "gh") {
		t.Fatalf("err = %v, want error pointing user at `gh`", err)
	}

	// And no event was appended.
	events, _ := st.Events.ListForStream(ctx, "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (publish must not append on rejection)", len(events))
	}
}

// TestPublishLocalStreamStillWorks: the rejection above must not
// regress publish to local streams.
func TestPublishLocalStreamStillWorks(t *testing.T) {
	t.Parallel()

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	stream, err := domain.NewStream("s-general", "s-general", "", "w-owner",
		time.Now().UTC(), domain.LocalTransport())
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	caller, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"}, "")

	deps := DefaultDeps(st)
	tool := &Publish{deps: deps}

	args, _ := json.Marshal(map[string]any{
		"streamId": "s-general",
		"body":     "hello",
	})
	if _, err := tool.Invoke(ctx, domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke = %v, want nil for local stream", err)
	}

	events, _ := st.Events.ListForStream(ctx, "s-general", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}
