package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestPublishRejectsGitHubStream: publishing to a github transport
// stream is rejected with an explanatory error rather than a silent
// no-op. GitHub streams are inbound-only; outbound action lives in
// the Worker's `gh`. See design/github-transport.md.
func TestPublishRejectsGitHubStream(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()

	// Seed a github-transport Stream and a caller Worker.
	cfg, _ := json.Marshal(map[string]any{
		"repo":   "helixml/helix-org",
		"events": []string{"issues"},
	})
	stream, err := streaming.NewStream("s-github", "s-github", "", "w-owner",
		time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	caller, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")

	deps := DefaultDeps(st)
	tl := &Publish{deps: deps}

	args, _ := json.Marshal(map[string]any{
		"streamId": "s-github",
		"body":     "this should be rejected",
	})

	_, err = tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args})
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
	events, _ := st.Events.ListForStream(ctx, "org-test", "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (publish must not append on rejection)", len(events))
	}
}

// TestPublishLocalStreamStillWorks: the rejection above must not
// regress publish to local streams.
func TestPublishLocalStreamStillWorks(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()

	stream, err := streaming.NewStream("s-general", "s-general", "", "w-owner",
		time.Now().UTC(), transport.LocalTransport(), "org-test")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	caller, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")

	deps := DefaultDeps(st)
	tl := &Publish{deps: deps}

	args, _ := json.Marshal(map[string]any{
		"streamId": "s-general",
		"body":     "hello",
	})
	if _, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke = %v, want nil for local stream", err)
	}

	events, _ := st.Events.ListForStream(ctx, "org-test", "s-general", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}
