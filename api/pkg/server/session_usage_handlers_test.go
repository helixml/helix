package server

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func TestBuildSessionUsageGroupsCallsIntoTurnsByTime(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, "2026-09-25T"+s+"Z")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// Bot sessions: calls carry no interaction id, and turn 1's `created` was
	// rewritten to turn 2's time — only `scheduled` marks when it started.
	interactions := []*types.Interaction{
		{ID: "int_02", Created: at("11:23:29"), Scheduled: at("11:23:29"), Completed: at("11:24:52"), PromptMessage: "code 199143", State: types.InteractionStateComplete},
		{ID: "int_01", Created: at("11:23:29"), Scheduled: at("11:20:39"), Completed: at("11:21:29"), PromptMessage: "Hi!", State: types.InteractionStateComplete},
	}
	calls := []*types.LLMCall{
		{Created: at("11:23:30"), DurationMs: 6400, TimeToFirstTokenMs: 3000, PromptTokens: 31638, CompletionTokens: 485, CacheReadTokens: 30000, Model: "glm-5.3-flash"},
		{Created: at("11:20:42"), DurationMs: 3500, TimeToFirstTokenMs: 700, PromptTokens: 641, CompletionTokens: 458, Model: "glm-5.3-flash"},
		{Created: at("11:21:05"), DurationMs: 5800, TimeToFirstTokenMs: 3700, PromptTokens: 29685, CompletionTokens: 292, CacheReadTokens: 28000, TotalCost: 0.01, Model: "glm-5.3-flash"},
		{Created: at("11:23:59"), InteractionID: "int_02", DurationMs: 21400, TimeToFirstTokenMs: 3500, PromptTokens: 43263, CompletionTokens: 4198, CacheReadTokens: 40000, Model: "qwen3.8-flash-next"},
	}
	u := buildSessionUsage("ses_1", calls, interactions)

	if u.Summary.Calls != 4 || u.Summary.PromptTokens != 105227 || u.Summary.LLMMs != 37100 {
		t.Fatalf("summary totals: %+v", u.Summary)
	}
	if r := *u.Summary.CacheHitRatio; r < 0.93 || r > 0.94 {
		t.Fatalf("cache hit ratio = %v, want 98000/105227", r)
	}
	if u.Summary.TTFTP50Ms != 3000 || u.Summary.TTFTP90Ms != 3700 || u.Summary.DurationP90Ms != 21400 {
		t.Fatalf("percentiles: %+v", u.Summary)
	}
	if len(u.Summary.Models) != 2 || u.Summary.Models[0] != "glm-5.3-flash" {
		t.Fatalf("models: %v", u.Summary.Models)
	}
	if len(u.Turns) != 2 || u.Turns[0].InteractionID != "int_01" || u.Turns[0].Calls != 2 || u.Turns[1].Calls != 2 {
		t.Fatalf("turns: %+v", u.Turns)
	}
	if !u.Turns[0].Started.Equal(at("11:20:39")) || u.Turns[0].CacheHitRatio == nil {
		t.Fatalf("turn 1 should start at scheduled time: %+v", u.Turns[0])
	}
	if !u.Calls[0].Created.Equal(at("11:20:42")) {
		t.Fatalf("calls not sorted by time: %v", u.Calls[0].Created)
	}
}

func TestBuildSessionUsageEmpty(t *testing.T) {
	u := buildSessionUsage("ses_1", nil, nil)
	if u.Summary.Calls != 0 || u.Summary.CacheHitRatio != nil || len(u.Calls) != 0 || len(u.Turns) != 0 {
		t.Fatalf("empty usage: %+v", u)
	}
}
