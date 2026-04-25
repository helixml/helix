package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
)

// TestListFeed seeds a Worker with a Stream on one Channel, appends two
// events, and confirms the endpoint returns them newest-first with correct
// attributes.
func TestListFeed(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	reg := tools.NewRegistry()
	srv := httptest.NewServer(server.New(s, reg, nil, nil, "").Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	role, _ := domain.NewRole("r", "# R", base)
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	pos, _ := domain.NewPosition("p", "r", nil)
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("seed pos: %v", err)
	}
	worker, _ := domain.NewAIWorker("w-1", []domain.PositionID{"p"})
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("seed worker: %v", err)
	}

	ch, _ := domain.NewChannel("c-general", "general", "", "w-1", base)
	if err := s.Channels.Create(ctx, ch); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	stream, _ := domain.NewStream("s-1", "w-1", "c-general", base)
	if err := s.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	for i, body := range []string{"one", "two"} {
		e, _ := domain.NewEvent(
			domain.EventID([]string{"e-a", "e-b"}[i]),
			"c-general",
			"w-1",
			body,
			base.Add(time.Duration(i+1)*time.Second),
		)
		if err := s.Events.Append(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	withGet(t, srv.URL+"/workers/w-1/feed", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var resources []server.Resource
		if err := json.Unmarshal(env.Data, &resources); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resources) != 2 {
			t.Fatalf("feed len = %d", len(resources))
		}
		if resources[0].ID != "e-b" || resources[1].ID != "e-a" {
			t.Fatalf("order: %+v", resources)
		}
	})

	withGet(t, srv.URL+"/workers/w-1/feed?limit=1", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var resources []server.Resource
		if err := json.Unmarshal(env.Data, &resources); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resources) != 1 || resources[0].ID != "e-b" {
			t.Fatalf("limit = %+v", resources)
		}
	})

	withGet(t, srv.URL+"/workers/w-1/feed?limit=0", func(res *http.Response, _ envelope) {
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("zero limit status = %d, want 400", res.StatusCode)
		}
	})

	withGet(t, srv.URL+"/workers/missing/feed", func(res *http.Response, _ envelope) {
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("missing worker status = %d, want 404", res.StatusCode)
		}
	})
}
