package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestPublishRejectsGitHubTopic: publishing to a github transport
// topic is rejected with an explanatory error rather than a silent
// no-op. GitHub topics are inbound-only; outbound action lives in
// the Worker's `gh`. See design/github-transport.md.
func TestPublishRejectsGitHubTopic(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()

	// Seed a github-transport Topic and a caller Worker.
	cfg, _ := json.Marshal(map[string]any{
		"repo":   "helixml/helix-org",
		"events": []string{"issues"},
	})
	topic, err := streaming.NewTopic("s-github", "s-github", "", "w-owner",
		time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test")
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	caller := botCaller{id: "b-owner", orgID: "org-test"}

	deps := DefaultDeps(st)
	tl := &Publish{deps: deps.Build()}

	args, _ := json.Marshal(map[string]any{
		"topicId": "s-github",
		"body":    "this should be rejected",
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
	events, _ := st.Events.ListForTopic(ctx, "org-test", "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (publish must not append on rejection)", len(events))
	}
}

// TestPublishLocalTopicStillWorks: the rejection above must not
// regress publish to local topics.
func TestPublishLocalTopicStillWorks(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()

	topic, err := streaming.NewTopic("s-general", "s-general", "", "w-owner",
		time.Now().UTC(), transport.LocalTransport(), "org-test")
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	caller := botCaller{id: "b-owner", orgID: "org-test"}

	deps := DefaultDeps(st)
	tl := &Publish{deps: deps.Build()}

	args, _ := json.Marshal(map[string]any{
		"topicId": "s-general",
		"body":    "hello",
	})
	if _, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke = %v, want nil for local topic", err)
	}

	events, _ := st.Events.ListForTopic(ctx, "org-test", "s-general", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}
