package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// sessionUsageMaxCalls caps how many llm_calls one usage response aggregates.
const sessionUsageMaxCalls = 2000

// getSessionUsage godoc
// @Summary Get a session's LLM usage
// @Description Spend, tokens, latency and prompt-cache hit ratio of one session, overall, per turn and per LLM call.
// @Tags    sessions
// @Success 200 {object} types.SessionUsage
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id}/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionUsage(_ http.ResponseWriter, req *http.Request) (*types.SessionUsage, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	session, err := s.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", id))
	}
	if err := s.authorizeUserToSession(ctx, user, session, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	calls, total, err := s.Store.ListLLMCalls(ctx, &store.ListLLMCallsQuery{
		SessionID: id,
		Page:      1,
		PerPage:   sessionUsageMaxCalls,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list LLM calls: %s", err))
	}
	interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    id,
		GenerationID: -1,
		PerPage:      1000,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list interactions: %s", err))
	}

	usage := buildSessionUsage(id, calls, interactions)
	usage.Truncated = total > int64(len(calls))
	return usage, nil
}

// buildSessionUsage aggregates a session's LLM calls overall and per turn.
// Calls are matched to turns by interaction id when the call has one; bot and
// external-agent calls usually don't, so those are matched by time: a turn runs
// from `scheduled` (its `created` can be rewritten later) to `completed`.
func buildSessionUsage(sessionID string, calls []*types.LLMCall, interactions []*types.Interaction) *types.SessionUsage {
	sort.Slice(calls, func(a, b int) bool { return calls[a].Created.Before(calls[b].Created) })
	sort.Slice(interactions, func(a, b int) bool { return interactions[a].ID < interactions[b].ID })

	usage := &types.SessionUsage{SessionID: sessionID, Calls: make([]types.SessionUsageCall, 0, len(calls))}
	turns := make([]types.SessionUsageTurn, len(interactions))
	byID := make(map[string]int, len(interactions))
	for i, in := range interactions {
		started := in.Scheduled
		if started.IsZero() {
			started = in.Created
		}
		turns[i] = types.SessionUsageTurn{
			InteractionID: in.ID,
			Prompt:        truncateRunes(in.PromptMessage, 200),
			Started:       started,
			Completed:     in.Completed,
			State:         string(in.State),
		}
		byID[in.ID] = i
	}
	turnFor := func(c *types.LLMCall) int {
		if i, ok := byID[c.InteractionID]; ok {
			return i
		}
		match := -1
		for i := range turns { // latest turn that started at or before the call
			if !turns[i].Started.After(c.Created.Add(time.Second)) {
				match = i
			}
		}
		return match
	}

	models := map[string]bool{}
	var ttfts, durations []int64
	sum := &usage.Summary
	for _, c := range calls {
		usage.Calls = append(usage.Calls, types.SessionUsageCall{
			Created:            c.Created,
			InteractionID:      c.InteractionID,
			Model:              c.Model,
			DurationMs:         c.DurationMs,
			TimeToFirstTokenMs: c.TimeToFirstTokenMs,
			PromptTokens:       c.PromptTokens,
			CompletionTokens:   c.CompletionTokens,
			CacheReadTokens:    c.CacheReadTokens,
			CacheWriteTokens:   c.CacheWriteTokens,
			TotalCost:          c.TotalCost,
		})
		sum.Calls++
		sum.PromptTokens += c.PromptTokens
		sum.CompletionTokens += c.CompletionTokens
		sum.CacheReadTokens += c.CacheReadTokens
		sum.CacheWriteTokens += c.CacheWriteTokens
		sum.TotalCost += c.TotalCost
		sum.LLMMs += c.DurationMs
		durations = append(durations, c.DurationMs)
		if c.TimeToFirstTokenMs > 0 {
			ttfts = append(ttfts, c.TimeToFirstTokenMs)
		}
		if c.Model != "" {
			models[c.Model] = true
		}
		if i := turnFor(c); i >= 0 {
			t := &turns[i]
			t.Calls++
			t.PromptTokens += c.PromptTokens
			t.CompletionTokens += c.CompletionTokens
			t.CacheReadTokens += c.CacheReadTokens
			t.LLMMs += c.DurationMs
			t.TotalCost += c.TotalCost
		}
	}
	sum.CacheHitRatio = ratio(sum.CacheReadTokens, sum.PromptTokens)
	sum.TTFTP50Ms, sum.TTFTP90Ms = percentile(ttfts, 0.5), percentile(ttfts, 0.9)
	sum.DurationP50Ms, sum.DurationP90Ms = percentile(durations, 0.5), percentile(durations, 0.9)
	sum.Models = make([]string, 0, len(models))
	for m := range models {
		sum.Models = append(sum.Models, m)
	}
	sort.Strings(sum.Models)
	for i := range turns {
		turns[i].CacheHitRatio = ratio(turns[i].CacheReadTokens, turns[i].PromptTokens)
	}
	usage.Turns = turns
	return usage
}

func ratio(part, whole int64) *float64 {
	if whole <= 0 {
		return nil
	}
	r := float64(part) / float64(whole)
	return &r
}

// percentile uses the nearest-rank method on a copy of xs; 0 for no samples.
func percentile(xs []int64, p float64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
	idx := int(p*float64(len(s)) + 0.5)
	if idx < 1 {
		idx = 1
	}
	if idx > len(s) {
		idx = len(s)
	}
	return s[idx-1]
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
