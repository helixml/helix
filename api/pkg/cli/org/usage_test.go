package org

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/helixml/helix/api/pkg/types"
)

func interactionAt(t *testing.T, start, completed time.Time, prompt string) *types.Interaction {
	t.Helper()
	id := ulid.MustNew(ulid.Timestamp(start), ulid.DefaultEntropy())
	// Like the API: a lower-case ULID, and `created` rewritten after the fact.
	return &types.Interaction{ID: "int_" + strings.ToLower(id.String()), Created: completed.Add(time.Hour), Completed: completed, PromptMessage: prompt}
}

func TestAggregateCallsPercentilesAndCacheRatio(t *testing.T) {
	var calls []*types.LLMCall
	for n := 1; n <= 10; n++ {
		calls = append(calls, &types.LLMCall{
			DurationMs: int64(n * 1000), TimeToFirstTokenMs: int64(n * 100),
			PromptTokens: 1000, CompletionTokens: 10, CacheReadTokens: 750, TotalCost: 0.001, Model: "m",
		})
	}
	calls = append(calls, &types.LLMCall{DurationMs: 500, PromptTokens: 1000, Model: "m2"}) // errored: no TTFT
	s := aggregateCalls(calls)
	if s.Calls != 11 || s.PromptTokens != 11000 || s.CacheReadTokens != 7500 {
		t.Fatalf("sums: %+v", s)
	}
	if math.Abs(s.CacheHitRatio-7500.0/11000) > 1e-9 {
		t.Fatalf("cache hit ratio = %v", s.CacheHitRatio)
	}
	// TTFT ignores the 0 of the errored call: 100..1000 ms.
	if s.TTFTP50Ms != 600 || s.TTFTP90Ms != 900 {
		t.Fatalf("ttft p50/p90 = %v/%v", s.TTFTP50Ms, s.TTFTP90Ms)
	}
	if s.DurationP50Ms != 5000 || s.DurationP90Ms != 9000 {
		t.Fatalf("duration p50/p90 = %v/%v", s.DurationP50Ms, s.DurationP90Ms)
	}
	if math.Abs(s.TotalCost-0.01) > 1e-9 || math.Abs(s.LLMSeconds-55.5) > 1e-9 {
		t.Fatalf("cost/time = %v/%v", s.TotalCost, s.LLMSeconds)
	}
	if strings.Join(s.Models, ",") != "m,m2" {
		t.Fatalf("models = %v", s.Models)
	}
	if empty := aggregateCalls(nil); empty.CacheHitRatio != 0 || empty.TTFTP50Ms != 0 {
		t.Fatalf("empty aggregate: %+v", empty)
	}
}

func TestGroupTurnsByTimeWindow(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 11, 20, 39, 0, time.UTC)
	first := interactionAt(t, t0, t0.Add(50*time.Second), "register me")
	second := interactionAt(t, t0.Add(170*time.Second), t0.Add(253*time.Second), "here is the OTP")
	running := interactionAt(t, t0.Add(400*time.Second), time.Time{}, "still going")
	at := func(d time.Duration, iid string) *types.LLMCall {
		return &types.LLMCall{Created: t0.Add(d), InteractionID: iid, PromptTokens: 1}
	}
	calls := []*types.LLMCall{
		at(3*time.Second, "n/a"),      // turn 1
		at(42*time.Second, ""),        // turn 1
		at(100*time.Second, ""),       // between turns 1 and 2
		at(171*time.Second, ""),       // turn 2
		at(60*time.Second, second.ID), // interaction id wins over time
		at(169*time.Second, ""),       // 1s before turn 2 starts: within the grace
		at(500*time.Second, ""),       // the running turn has no end
	}
	turns, outside := groupTurns(calls, []*types.Interaction{running, second, first})
	if len(turns) != 3 || turns[0].Prompt != "register me" || turns[2].Prompt != "still going" {
		t.Fatalf("turns not ordered by id: %+v", turns)
	}
	if got := []int{turns[0].Calls, turns[1].Calls, turns[2].Calls}; got[0] != 2 || got[1] != 3 || got[2] != 1 {
		t.Fatalf("calls per turn = %v, want [2 3 1]", got)
	}
	if len(outside) != 1 || !outside[0].Created.Equal(t0.Add(100*time.Second)) {
		t.Fatalf("outside = %+v", outside)
	}
	if !turns[0].Start.Equal(t0) {
		t.Fatalf("turn start should come from the ULID, got %v", turns[0].Start)
	}
}
