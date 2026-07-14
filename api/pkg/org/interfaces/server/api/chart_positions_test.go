package api_test

import (
	"net/http"
	"testing"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

func TestChartPositions_ListEmpty(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "GET", "/chart/positions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp orgapi.ChartPositionsResponse
	decode(t, rec, &resp)
	if len(resp.Positions) != 0 {
		t.Fatalf("expected empty positions, got %+v", resp.Positions)
	}
}

func TestChartPositions_UpsertListClear(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	// Upsert two nodes.
	rec := do(t, h, "PUT", "/chart/positions", orgapi.UpsertChartPositionsRequest{
		Positions: []orgapi.ChartPositionDTO{
			{Kind: "bot", ID: "b-owner", X: 120, Y: 40},
			{Kind: "topic", ID: "s-inbox", X: 400, Y: 80},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}
	var putResp orgapi.ChartPositionsResponse
	decode(t, rec, &putResp)
	if len(putResp.Positions) != 2 {
		t.Fatalf("put returned %d positions, want 2", len(putResp.Positions))
	}

	// List returns both.
	rec = do(t, h, "GET", "/chart/positions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list orgapi.ChartPositionsResponse
	decode(t, rec, &list)
	if len(list.Positions) != 2 {
		t.Fatalf("list returned %d, want 2: %+v", len(list.Positions), list.Positions)
	}
	byKey := map[string]orgapi.ChartPositionDTO{}
	for _, p := range list.Positions {
		byKey[p.Kind+":"+p.ID] = p
	}
	if p := byKey["bot:b-owner"]; p.X != 120 || p.Y != 40 {
		t.Fatalf("bot position = %+v, want x=120 y=40", p)
	}
	if p := byKey["topic:s-inbox"]; p.X != 400 || p.Y != 80 {
		t.Fatalf("topic position = %+v, want x=400 y=80", p)
	}

	// Re-drag the bot — replaces coordinates.
	rec = do(t, h, "PUT", "/chart/positions", orgapi.UpsertChartPositionsRequest{
		Positions: []orgapi.ChartPositionDTO{
			{Kind: "bot", ID: "b-owner", X: 10, Y: 20},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-put status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/chart/positions", nil)
	decode(t, rec, &list)
	byKey = map[string]orgapi.ChartPositionDTO{}
	for _, p := range list.Positions {
		byKey[p.Kind+":"+p.ID] = p
	}
	if p := byKey["bot:b-owner"]; p.X != 10 || p.Y != 20 {
		t.Fatalf("bot after re-drag = %+v, want x=10 y=20", p)
	}
	if _, ok := byKey["topic:s-inbox"]; !ok {
		t.Fatal("topic position was wiped by bot-only upsert")
	}

	// Clear resets to empty.
	rec = do(t, h, "DELETE", "/chart/positions", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/chart/positions", nil)
	decode(t, rec, &list)
	if len(list.Positions) != 0 {
		t.Fatalf("after clear expected empty, got %+v", list.Positions)
	}
}

func TestChartPositions_RejectsInvalidKind(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "PUT", "/chart/positions", orgapi.UpsertChartPositionsRequest{
		Positions: []orgapi.ChartPositionDTO{
			{Kind: "person", ID: "p-1", X: 1, Y: 2},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}
