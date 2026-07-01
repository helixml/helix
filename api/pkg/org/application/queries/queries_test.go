package queries

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// TestQueries_ReadsAcrossAggregates exercises the read facade end to end
// against the in-memory store: seed a bot, topic, subscription, and
// event, then read each back through Queries.
func TestQueries_ReadsAcrossAggregates(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	b, _ := orgchart.NewBot("w-mark", "# Eng", []tool.Name{"publish"}, now, "org-test")
	if err := st.Bots.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	topic, _ := streaming.NewTopic("s-1", "s-1", "", "w-owner", now, transport.LocalTransport(), "org-test")
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatal(err)
	}
	sub, _ := streaming.NewSubscription("w-mark", "s-1", now, "org-test")
	if err := st.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	ev, _ := streaming.NewMessageEvent("e-1", "s-1", "w-mark", streaming.Message{Body: "hi"}, now, "org-test")
	if err := st.Events.Append(ctx, ev); err != nil {
		t.Fatal(err)
	}

	q := New(Deps{
		Bots: st.Bots, ReportingLines: st.ReportingLines,
		Topics: st.Topics, Subscriptions: st.Subscriptions, Events: st.Events,
	})

	if bots, err := q.ListBots(ctx, "org-test"); err != nil || len(bots) != 1 {
		t.Fatalf("ListBots = %v, %v", bots, err)
	}
	if got, err := q.GetBot(ctx, "org-test", "w-mark"); err != nil || got.ID != "w-mark" {
		t.Fatalf("GetBot = %v, %v", got, err)
	}
	if topics, err := q.ListTopics(ctx, "org-test"); err != nil || len(topics) != 1 {
		t.Fatalf("ListTopics = %v, %v", topics, err)
	}
	if subs, err := q.TopicSubscribers(ctx, "org-test", "s-1"); err != nil || len(subs) != 1 {
		t.Fatalf("TopicSubscribers = %v, %v", subs, err)
	}
	if subs, err := q.BotSubscriptions(ctx, "org-test", "w-mark"); err != nil || len(subs) != 1 {
		t.Fatalf("BotSubscriptions = %v, %v", subs, err)
	}
	if events, err := q.TopicEvents(ctx, "org-test", "s-1", 10); err != nil || len(events) != 1 {
		t.Fatalf("TopicEvents = %v, %v", events, err)
	}
	if events, err := q.AllEvents(ctx, "org-test", 10); err != nil || len(events) != 1 {
		t.Fatalf("AllEvents = %v, %v", events, err)
	}
	if !q.ReportingLinesWired() {
		t.Fatal("ReportingLinesWired should be true")
	}
}
