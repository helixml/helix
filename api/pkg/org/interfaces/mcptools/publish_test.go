package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

func injectTestPublishing(cfg *Config) {
	deps := publishing.Deps{
		Topics:     cfg.Store.Topics,
		Events:     cfg.Store.Events,
		Dispatcher: cfg.Dispatcher,
		Now:        cfg.Now,
		NewID:      cfg.NewID,
	}
	if cfg.Hub != nil {
		deps.Hub = cfg.Hub
	}
	cfg.Publishing = publishing.New(deps)
}

func TestRegisterBuiltinsRequiresPublishing(t *testing.T) {
	deps := DefaultDeps(orggorm.GetOrgTestDB(t)).Build()
	err := RegisterBuiltins(NewRegistry(), deps)
	if err == nil || !strings.Contains(err.Error(), "deps.Publishing is required") {
		t.Fatalf("RegisterBuiltins error = %v", err)
	}
}

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
	injectTestPublishing(&deps)
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
	injectTestPublishing(&deps)
	tl := &Publish{deps: deps.Build()}

	args, _ := json.Marshal(map[string]any{
		"topicId": "s-general",
		"body":    "hello",
	})
	raw, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args})
	if err != nil {
		t.Fatalf("Invoke = %v, want nil for local topic", err)
	}
	var result struct {
		ID       string `json:"id"`
		TopicID  string `json:"topicId"`
		Scope    string `json:"scope"`
		Status   string `json:"status"`
		Delivery struct {
			Status   string `json:"status"`
			Provider string `json:"provider"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID == "" || result.TopicID != "s-general" || result.Scope != "helix" || result.Status != "appended inside Helix; external delivery not confirmed" || result.Delivery.Status != "not_applicable" || result.Delivery.Provider != "helix" {
		t.Fatalf("result = %#v", result)
	}

	events, _ := st.Events.ListForTopic(ctx, "org-test", "s-general", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

type fakePublishDeliverer struct{}

func (fakePublishDeliverer) Deliver(context.Context, streaming.Topic, streaming.Message) (publishing.DeliveryReceipt, error) {
	return publishing.DeliveryReceipt{Status: "delivered", Provider: "slack", Destination: "C123", MessageID: "1.2"}, nil
}

func TestPublishSlackTopicPreservesContractAndReportsDelivery(t *testing.T) {
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	cfg, _ := json.Marshal(transport.SlackConfig{ServiceConnectionID: "sc-1", ChannelID: "C123"})
	topic, err := streaming.NewTopic("s-slack", "s-slack", "", "b-owner", time.Now().UTC(), transport.Transport{Kind: transport.KindSlack, Config: cfg}, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatal(err)
	}
	toolCfg := DefaultDeps(st)
	injectTestPublishing(&toolCfg)
	toolCfg.Publishing.RegisterDeliverer(transport.KindSlack, fakePublishDeliverer{})
	args, _ := json.Marshal(map[string]any{"topicId": "s-slack", "body": "hello"})
	raw, err := (&Publish{deps: toolCfg.Build()}).Invoke(ctx, tool.Invocation{Caller: botCaller{id: "b-owner", orgID: "org-test"}, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID, TopicID, Scope, Status string
		Delivery                   publishing.DeliveryReceipt `json:"delivery"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || result.TopicID != "s-slack" || result.Scope != "helix" || result.Status != "appended inside Helix and delivered to slack" {
		t.Fatalf("result = %#v", result)
	}
	if result.Delivery.Status != "delivered" || result.Delivery.Destination != "C123" || result.Delivery.MessageID != "1.2" {
		t.Fatalf("delivery = %#v", result.Delivery)
	}
}

func TestPublishDescriptionExplainsSlackDeliveryBoundary(t *testing.T) {
	description := (&Publish{}).Description()
	for _, want := range []string{"through Helix", "configured Slack Topic", "delivery receipt", "ask_human", "mint_credential", "reactions", "uploads", "edits"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description %q missing %q", description, want)
		}
	}
}
