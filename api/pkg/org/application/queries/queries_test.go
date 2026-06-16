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
// against the in-memory store: seed a role, worker, stream, subscription,
// and event, then read each back through Queries.
func TestQueries_ReadsAcrossAggregates(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, now, "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	wk, _ := orgchart.NewAIWorker("w-mark", "r-eng", "id", "org-test")
	if err := st.Workers.Create(ctx, wk); err != nil {
		t.Fatal(err)
	}
	stream, _ := streaming.NewStream("s-1", "s-1", "", "w-owner", now, transport.LocalTransport(), "org-test")
	if err := st.Streams.Create(ctx, stream); err != nil {
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
		Roles: st.Roles, Workers: st.Workers, ReportingLines: st.ReportingLines,
		Streams: st.Streams, Subscriptions: st.Subscriptions, Events: st.Events,
	})

	if roles, err := q.ListRoles(ctx, "org-test"); err != nil || len(roles) != 1 {
		t.Fatalf("ListRoles = %v, %v", roles, err)
	}
	if got, err := q.GetWorker(ctx, "org-test", "w-mark"); err != nil || got.ID() != "w-mark" {
		t.Fatalf("GetWorker = %v, %v", got, err)
	}
	if streams, err := q.ListStreams(ctx, "org-test"); err != nil || len(streams) != 1 {
		t.Fatalf("ListStreams = %v, %v", streams, err)
	}
	if subs, err := q.StreamSubscribers(ctx, "org-test", "s-1"); err != nil || len(subs) != 1 {
		t.Fatalf("StreamSubscribers = %v, %v", subs, err)
	}
	if subs, err := q.WorkerSubscriptions(ctx, "org-test", "w-mark"); err != nil || len(subs) != 1 {
		t.Fatalf("WorkerSubscriptions = %v, %v", subs, err)
	}
	if events, err := q.StreamEvents(ctx, "org-test", "s-1", 10); err != nil || len(events) != 1 {
		t.Fatalf("StreamEvents = %v, %v", events, err)
	}
	if events, err := q.AllEvents(ctx, "org-test", 10); err != nil || len(events) != 1 {
		t.Fatalf("AllEvents = %v, %v", events, err)
	}
	if !q.ReportingLinesWired() {
		t.Fatal("ReportingLinesWired should be true")
	}
}
