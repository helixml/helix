package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

func seedStreamAndOwner(t *testing.T, st *store.Store, clock func() time.Time) {
	t.Helper()
	ctx := context.Background()
	s, _ := streaming.NewStream("s-1", "s-1", "", "w-owner", clock(), transport.LocalTransport(), "org-test")
	if err := st.Streams.Create(ctx, s); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	wk, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	if err := st.Workers.Create(ctx, wk); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
}

// TestSubscribeParity_RESTvsMCP: REST POST /workers/{id}/subscriptions
// and MCP subscribe share application/subscriptions — both leave the
// same (worker, stream) row.
func TestSubscribeParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }
	ctx := context.Background()

	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	seedStreamAndOwner(t, restStore, clock)
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/workers/w-owner/subscriptions", orgapi.SubscribeWorkerRequest{StreamID: "s-1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST subscribe: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	seedStreamAndOwner(t, mcpStore, clock)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	subscribe, _ := reg.Get(mcptools.SubscribeName)
	args, _ := json.Marshal(map[string]any{"streamId": "s-1"})
	if _, err := subscribe.Invoke(ctx, tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP subscribe: %v", err)
	}

	restSub, err := restStore.Subscriptions.Find(ctx, "org-test", "w-owner", "s-1")
	if err != nil {
		t.Fatalf("REST sub find: %v", err)
	}
	mcpSub, err := mcpStore.Subscriptions.Find(ctx, "org-test", "w-owner", "s-1")
	if err != nil {
		t.Fatalf("MCP sub find: %v", err)
	}
	if restSub.WorkerID != mcpSub.WorkerID || restSub.StreamID != mcpSub.StreamID {
		t.Errorf("sub differs: REST=%+v MCP=%+v", restSub, mcpSub)
	}
	if !restSub.CreatedAt.Equal(mcpSub.CreatedAt) {
		t.Errorf("CreatedAt differs: REST=%v MCP=%v", restSub.CreatedAt, mcpSub.CreatedAt)
	}
}

// TestPublishParity_RESTvsMCP: REST publish and MCP publish share
// application/publishing — both append an identical Message event.
func TestPublishParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }
	ctx := context.Background()

	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	seedStreamAndOwner(t, restStore, clock)
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/streams/s-1/publish", orgapi.PublishRequest{Body: "hello world", Subject: "hi", As: "w-owner"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST publish: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	seedStreamAndOwner(t, mcpStore, clock)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	publish, _ := reg.Get(mcptools.PublishName)
	args, _ := json.Marshal(map[string]any{"streamId": "s-1", "body": "hello world", "subject": "hi"})
	if _, err := publish.Invoke(ctx, tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP publish: %v", err)
	}

	restEvents, _ := restStore.Events.ListForStream(ctx, "org-test", "s-1", 10)
	mcpEvents, _ := mcpStore.Events.ListForStream(ctx, "org-test", "s-1", 10)
	if len(restEvents) != 1 || len(mcpEvents) != 1 {
		t.Fatalf("event counts: REST=%d MCP=%d, want 1 each", len(restEvents), len(mcpEvents))
	}
	rm, _ := restEvents[0].Message()
	mm, _ := mcpEvents[0].Message()
	if rm.From != mm.From || rm.Body != mm.Body || rm.Subject != mm.Subject {
		t.Errorf("message differs: REST=%+v MCP=%+v", rm, mm)
	}
	if restEvents[0].Source != mcpEvents[0].Source {
		t.Errorf("source differs: REST=%q MCP=%q", restEvents[0].Source, mcpEvents[0].Source)
	}
	if rm.From != "w-owner" {
		t.Errorf("From = %q, want w-owner", rm.From)
	}
}
