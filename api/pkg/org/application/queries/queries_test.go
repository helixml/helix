package queries

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

// TestQueries_ReadsAcrossAggregates exercises the read facade end to end
// against the in-memory store: seed a Worker, a Trigger, an attachment
// and an event, then read each back through Queries.
func TestQueries_ReadsAcrossAggregates(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	b, _ := orgchart.NewNode("w-mark", "# Eng", []tool.Name{"chat"}, now, "org-test")
	if err := st.Nodes.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	orgtest.Trigger(t, st, "org-test", "s-1")
	orgtest.AttachTrigger(t, st, "org-test", "w-mark", "s-1")
	ev, _ := streaming.NewMessageEvent("e-1", "s-1", "w-mark", streaming.Message{Body: "hi"}, now, "org-test")
	if err := st.Events.Append(ctx, ev); err != nil {
		t.Fatal(err)
	}

	q := New(Deps{
		Nodes: st.Nodes, ReportingLines: st.ReportingLines,
		Triggers: st.Triggers, Attachments: st.WorkerAttachments,
		Processors: st.Processors, Events: st.Events,
	})

	if bots, err := q.ListBots(ctx, "org-test"); err != nil || len(bots) != 1 {
		t.Fatalf("ListBots = %v, %v", bots, err)
	}
	if got, err := q.GetBot(ctx, "org-test", "w-mark"); err != nil || got.ID != "w-mark" {
		t.Fatalf("GetBot = %v, %v", got, err)
	}
	if rows, err := q.ListTriggers(ctx, "org-test"); err != nil || len(rows) != 1 {
		t.Fatalf("ListTriggers = %v, %v", rows, err)
	}
	if got, err := q.GetTrigger(ctx, "org-test", "s-1"); err != nil || got.ID != "s-1" {
		t.Fatalf("GetTrigger = %v, %v", got, err)
	}
	if members, err := q.TriggerMembers(ctx, "org-test", "s-1"); err != nil || len(members) != 1 {
		t.Fatalf("TriggerMembers = %v, %v", members, err)
	}
	if rows, err := q.WorkerAttachments(ctx, "org-test", "w-mark"); err != nil || len(rows) != 1 {
		t.Fatalf("WorkerAttachments = %v, %v", rows, err)
	}
	if _, err := q.FindAttachment(ctx, "org-test", "w-mark", "s-1"); err != nil {
		t.Fatalf("FindAttachment: %v", err)
	}
	if events, err := q.StreamEvents(ctx, "org-test", "s-1", 10); err != nil || len(events) != 1 {
		t.Fatalf("StreamEvents = %v, %v", events, err)
	}
	if events, err := q.BotEvents(ctx, "org-test", "w-mark", 10); err != nil || len(events) != 1 {
		t.Fatalf("BotEvents = %v, %v", events, err)
	}
	if events, err := q.AllEvents(ctx, "org-test", 10); err != nil || len(events) != 1 {
		t.Fatalf("AllEvents = %v, %v", events, err)
	}
	if !q.ReportingLinesWired() {
		t.Fatal("ReportingLinesWired should be true")
	}
}

// TestWorkerStreams_ResolvesProcessorBranches: a Worker attached to a
// Processor branch reads the stream recorded on that branch, not the
// branch id — the indirection that keeps a converted branch's history
// reachable.
func TestWorkerStreams_ResolvesProcessorBranches(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	b, _ := orgchart.NewNode("w-mark", "# Eng", nil, now, "org-test")
	if err := st.Nodes.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	p, err := processor.NewProcessor("p-1", "Router", eventsource.Trigger("s-in"), processor.KindTruncate,
		[]byte(`{"max_bytes":10}`),
		[]processor.Output{{ID: "po-vip", StreamID: "s-legacy-out"}},
		"", now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Processors.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	orgtest.Attach(t, st, "org-test", "w-mark", eventsource.ProcessorOutput("p-1", "po-vip"))

	q := New(Deps{Nodes: st.Nodes, Triggers: st.Triggers, Attachments: st.WorkerAttachments, Processors: st.Processors, Events: st.Events})
	streams, err := q.WorkerStreams(ctx, "org-test", "w-mark")
	if err != nil {
		t.Fatalf("WorkerStreams: %v", err)
	}
	if len(streams) != 1 || streams[0] != "s-legacy-out" {
		t.Fatalf("streams = %v, want [s-legacy-out]", streams)
	}
}

// TestWorkerStreams_SkipsDanglingAttachments: an attachment whose source
// is gone must not fail the Worker's whole inbox.
func TestWorkerStreams_SkipsDanglingAttachments(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	b, _ := orgchart.NewNode("w-mark", "# Eng", nil, now, "org-test")
	if err := st.Nodes.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	orgtest.Attach(t, st, "org-test", "w-mark", eventsource.ProcessorOutput("p-gone", "po-gone"))
	orgtest.Trigger(t, st, "org-test", "s-live")
	orgtest.AttachTrigger(t, st, "org-test", "w-mark", "s-live")

	q := New(Deps{Nodes: st.Nodes, Triggers: st.Triggers, Attachments: st.WorkerAttachments, Processors: st.Processors, Events: st.Events})
	streams, err := q.WorkerStreams(ctx, "org-test", "w-mark")
	if err != nil {
		t.Fatalf("WorkerStreams: %v", err)
	}
	if len(streams) != 1 || streams[0] != "s-live" {
		t.Fatalf("streams = %v, want [s-live]", streams)
	}
}
