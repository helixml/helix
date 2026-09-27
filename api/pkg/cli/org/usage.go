package org

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// `helix session usage`: tokens, cost, latency and cache hits of a session's
// LLM calls, in total and per turn. Calls come from the session's app
// (/agents/{app}/llm-calls?session=); bot sessions often log them without an
// interaction id, so calls are matched to turns by time.

// usageStats aggregates a set of LLM calls.
type usageStats struct {
	Calls            int      `json:"calls"`
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CacheHitRatio    float64  `json:"cache_hit_ratio"` // cache_read / prompt
	PromptCost       float64  `json:"prompt_cost"`
	CompletionCost   float64  `json:"completion_cost"`
	CacheReadCost    float64  `json:"cache_read_cost"`
	CacheWriteCost   float64  `json:"cache_write_cost"`
	TotalCost        float64  `json:"total_cost"`
	LLMSeconds       float64  `json:"llm_seconds"`
	TTFTP50Ms        float64  `json:"ttft_p50_ms"`
	TTFTP90Ms        float64  `json:"ttft_p90_ms"`
	DurationP50Ms    float64  `json:"duration_p50_ms"`
	DurationP90Ms    float64  `json:"duration_p90_ms"`
	Models           []string `json:"models"`
}

// turnUsage is the usage of one turn (interaction).
type turnUsage struct {
	InteractionID string    `json:"interaction_id"`
	Prompt        string    `json:"prompt"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	usageStats
	calls []*types.LLMCall
}

type sessionUsage struct {
	SessionID string      `json:"session_id"`
	AppID     string      `json:"app_id"`
	Total     usageStats  `json:"total"`
	Turns     []turnUsage `json:"turns"`
	// Outside holds calls in no turn's window (e.g. between turns).
	Outside *usageStats      `json:"outside_turns,omitempty"`
	Calls   []*types.LLMCall `json:"calls"`
}

func aggregateCalls(calls []*types.LLMCall) usageStats {
	var s usageStats
	var ttft, dur []float64
	models := map[string]bool{}
	for _, c := range calls {
		s.Calls++
		s.PromptTokens += c.PromptTokens
		s.CompletionTokens += c.CompletionTokens
		s.CacheReadTokens += c.CacheReadTokens
		s.CacheWriteTokens += c.CacheWriteTokens
		s.PromptCost += c.PromptCost
		s.CompletionCost += c.CompletionCost
		s.CacheReadCost += c.CacheReadCost
		s.CacheWriteCost += c.CacheWriteCost
		s.TotalCost += c.TotalCost
		s.LLMSeconds += float64(c.DurationMs) / 1000
		dur = append(dur, float64(c.DurationMs))
		if c.TimeToFirstTokenMs > 0 { // 0: no chunk arrived (errored or cut)
			ttft = append(ttft, float64(c.TimeToFirstTokenMs))
		}
		if c.Model != "" {
			models[c.Model] = true
		}
	}
	if s.PromptTokens > 0 {
		s.CacheHitRatio = float64(s.CacheReadTokens) / float64(s.PromptTokens)
	}
	s.TTFTP50Ms, s.TTFTP90Ms = pct(ttft, .5), pct(ttft, .9)
	s.DurationP50Ms, s.DurationP90Ms = pct(dur, .5), pct(dur, .9)
	s.Models = make([]string, 0, len(models))
	for m := range models {
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)
	return s
}

// turnStart is when a turn began. Interaction ids are ULIDs, whose timestamp
// is the creation time; `created` itself can be rewritten later.
func turnStart(i *types.Interaction) time.Time {
	id := i.ID
	if n := strings.LastIndex(id, "_"); n >= 0 {
		id = id[n+1:]
	}
	if u, err := ulid.Parse(strings.ToUpper(id)); err == nil {
		return ulid.Time(u.Time())
	}
	return i.Created
}

// turnGrace absorbs clock skew between the call log and the interaction rows.
const turnGrace = 2 * time.Second

// groupTurns assigns calls to turns: by interaction id when the call has a
// real one, else by `created` (the call's start) inside the turn's
// [start, completed] window. Calls in no window are returned as outside.
func groupTurns(calls []*types.LLMCall, xs []*types.Interaction) ([]turnUsage, []*types.LLMCall) {
	sorted := append([]*types.Interaction{}, xs...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].ID < sorted[b].ID })
	turns := make([]turnUsage, len(sorted))
	byID := map[string]int{}
	for n, i := range sorted {
		turns[n] = turnUsage{InteractionID: i.ID, Prompt: i.PromptMessage, Start: turnStart(i)}
		byID[i.ID] = n
	}
	for n, i := range sorted {
		end := time.Time{} // zero: open (still running)
		if !i.Completed.IsZero() && i.Completed.After(turns[n].Start) {
			end = i.Completed
		}
		if n+1 < len(turns) && (end.IsZero() || end.After(turns[n+1].Start)) {
			end = turns[n+1].Start
		}
		turns[n].End = end
	}
	var outside []*types.LLMCall
	for _, c := range calls {
		if n, ok := byID[c.InteractionID]; ok {
			turns[n].calls = append(turns[n].calls, c)
			continue
		}
		matched := false
		for n := len(turns) - 1; n >= 0; n-- {
			if c.Created.Before(turns[n].Start.Add(-turnGrace)) {
				continue
			}
			if turns[n].End.IsZero() || !c.Created.After(turns[n].End.Add(turnGrace)) {
				turns[n].calls = append(turns[n].calls, c)
				matched = true
			}
			break
		}
		if !matched {
			outside = append(outside, c)
		}
	}
	for n := range turns {
		turns[n].usageStats = aggregateCalls(turns[n].calls)
	}
	return turns, outside
}

func buildSessionUsage(sessionID, appID string, calls []*types.LLMCall, xs []*types.Interaction) *sessionUsage {
	u := &sessionUsage{SessionID: sessionID, AppID: appID, Total: aggregateCalls(calls), Calls: calls}
	var outside []*types.LLMCall
	u.Turns, outside = groupTurns(calls, xs)
	if len(outside) > 0 {
		s := aggregateCalls(outside)
		u.Outside = &s
	}
	return u
}

// fetchSessionUsage reads every LLM call and every turn of the session.
func fetchSessionUsage(ctx context.Context, c *client.HelixClient, sessionID string) (*sessionUsage, error) {
	session, err := c.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.ParentApp == "" {
		return nil, fmt.Errorf("session %s has no app, so it has no LLM call log", sessionID)
	}
	var calls []*types.LLMCall
	for page := 1; ; page++ { // llm-calls pages are 1-based
		resp, err := c.ListAppLLMCalls(ctx, session.ParentApp, &client.LLMCallFilter{SessionID: sessionID, Page: page, PageSize: 50})
		if err != nil {
			return nil, err
		}
		calls = append(calls, resp.Calls...)
		if len(resp.Calls) == 0 || page >= resp.TotalPages {
			break
		}
	}
	var xs []*types.Interaction
	for page := 0; ; page++ { // interaction pages are 0-based
		resp, err := c.ListInteractions(ctx, sessionID, &client.InteractionFilter{Page: page, PerPage: 100, Order: "asc"})
		if err != nil {
			return nil, err
		}
		xs = append(xs, resp.Interactions...)
		if len(resp.Interactions) == 0 || int64(len(xs)) >= resp.TotalCount {
			break
		}
	}
	for _, call := range calls { // bodies are the full prompts: keep them out of the output
		call.OriginalRequest, call.Request, call.Response = nil, nil, nil
	}
	return buildSessionUsage(sessionID, session.ParentApp, calls, xs), nil
}

// formatCost prints a cost, or n/a when the session has no cost data at all
// (providers without pricing log 0).
func formatCost(cost float64, priced bool) string {
	if !priced {
		return "n/a"
	}
	return fmt.Sprintf("$%.4f", cost)
}

// truncateRunes is truncate for table columns: it cuts on rune boundaries so
// non-ASCII prompts keep the columns aligned.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func printSessionUsage(u *sessionUsage, showCalls bool) {
	t := u.Total
	fmt.Printf("session %s  app %s\n", u.SessionID, u.AppID)
	fmt.Printf("  LLM calls          %d\n", t.Calls)
	fmt.Printf("  prompt tokens      %d\n", t.PromptTokens)
	fmt.Printf("  completion tokens  %d\n", t.CompletionTokens)
	fmt.Printf("  cache-read tokens  %d  (cache hit %.1f%% of prompt tokens)\n", t.CacheReadTokens, 100*t.CacheHitRatio)
	if t.CacheWriteTokens > 0 {
		fmt.Printf("  cache-write tokens %d\n", t.CacheWriteTokens)
	}
	priced := t.TotalCost != 0
	fmt.Printf("  cost               %s\n", formatCost(t.TotalCost, priced))
	fmt.Printf("  LLM time           %.1fs\n", t.LLMSeconds)
	fmt.Printf("  TTFT p50/p90       %.1fs / %.1fs\n", t.TTFTP50Ms/1000, t.TTFTP90Ms/1000)
	fmt.Printf("  call p50/p90       %.1fs / %.1fs\n", t.DurationP50Ms/1000, t.DurationP90Ms/1000)
	fmt.Printf("  models             %s\n", strings.Join(t.Models, ", "))
	fmt.Println()
	row := "%-5s %-40s %6s %11s %10s %7s %7s %9s\n"
	fmt.Printf(row, "TURN", "PROMPT", "CALLS", "PROMPT TOK", "COMPL TOK", "CACHE", "LLM s", "COST")
	line := func(label, prompt string, s usageStats) {
		fmt.Printf(row, label, truncateRunes(strings.Join(strings.Fields(prompt), " "), 40), fmt.Sprint(s.Calls),
			fmt.Sprint(s.PromptTokens), fmt.Sprint(s.CompletionTokens), fmt.Sprintf("%.0f%%", 100*s.CacheHitRatio),
			fmt.Sprintf("%.1f", s.LLMSeconds), formatCost(s.TotalCost, priced))
	}
	for n, turn := range u.Turns {
		line(fmt.Sprint(n+1), turn.Prompt, turn.usageStats)
	}
	if u.Outside != nil {
		line("-", "(outside any turn)", *u.Outside)
	}
	if !showCalls {
		return
	}
	fmt.Println()
	callRow := "%-19s %8s %7s %8s %7s %8s %9s  %s\n"
	fmt.Printf(callRow, "TIME", "DURATION", "TTFT", "IN", "OUT", "CACHE", "COST", "MODEL")
	for _, c := range u.Calls {
		fmt.Printf(callRow, c.Created.Local().Format("2006-01-02 15:04:05"), fmt.Sprintf("%.1fs", float64(c.DurationMs)/1000),
			fmt.Sprintf("%.1fs", float64(c.TimeToFirstTokenMs)/1000), fmt.Sprint(c.PromptTokens), fmt.Sprint(c.CompletionTokens),
			fmt.Sprint(c.CacheReadTokens), formatCost(c.TotalCost, priced), c.Model)
	}
}

func newSessionUsageCmd() *cobra.Command {
	var (
		calls   bool
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "usage <session-id>",
		Short: "Token usage, cost, latency and cache hits of a session, per turn",
		Long: `Summarise a session's LLM calls (bot main sessions and instances alike): calls, prompt,
completion and cache-read tokens, cache hit ratio (cache-read / prompt tokens), cost, LLM time,
time-to-first-token and call duration p50/p90, models; then one row per turn.

--calls lists every call; --json prints the aggregates and the calls (without request and
response bodies).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			// Call rows carry full prompts, so pages are large.
			ctx, cancel := callCtx(cmd.Context(), 2*time.Minute)
			defer cancel()
			u, err := fetchSessionUsage(ctx, c, args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(u)
			}
			printSessionUsage(u, calls)
			return nil
		},
	}
	cmd.Flags().BoolVar(&calls, "calls", false, "Also list every LLM call")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output (aggregates and calls)")
	return cmd
}
