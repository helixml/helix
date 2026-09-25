package types

import "time"

// SessionUsage is the LLM spend, token, latency and prompt-cache picture of one
// session (a chat, a spec task's agent, a bot's main session or a bot instance),
// built from its llm_calls rows. Returned by GET /api/v1/sessions/{id}/usage.
type SessionUsage struct {
	SessionID string              `json:"session_id"`
	Summary   SessionUsageSummary `json:"summary"`
	Calls     []SessionUsageCall  `json:"calls"`
	Turns     []SessionUsageTurn  `json:"turns"`
	// Truncated is set when the session has more calls than the endpoint returns;
	// the summary then covers only the returned calls.
	Truncated bool `json:"truncated,omitempty"`
}

type SessionUsageSummary struct {
	Calls            int   `json:"calls"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	// CacheHitRatio is cache-read / prompt tokens; nil when there were no prompt tokens.
	CacheHitRatio *float64 `json:"cache_hit_ratio"`
	TotalCost     float64  `json:"total_cost"`
	// LLMMs is the summed duration of the calls (calls can overlap, so this can exceed wall time).
	LLMMs         int64    `json:"llm_ms"`
	TTFTP50Ms     int64    `json:"ttft_p50_ms"`
	TTFTP90Ms     int64    `json:"ttft_p90_ms"`
	DurationP50Ms int64    `json:"duration_p50_ms"`
	DurationP90Ms int64    `json:"duration_p90_ms"`
	Models        []string `json:"models"`
}

type SessionUsageCall struct {
	Created            time.Time `json:"created"`
	InteractionID      string    `json:"interaction_id,omitempty"`
	Model              string    `json:"model"`
	DurationMs         int64     `json:"duration_ms"`
	TimeToFirstTokenMs int64     `json:"time_to_first_token_ms"`
	PromptTokens       int64     `json:"prompt_tokens"`
	CompletionTokens   int64     `json:"completion_tokens"`
	CacheReadTokens    int64     `json:"cache_read_tokens"`
	CacheWriteTokens   int64     `json:"cache_write_tokens"`
	TotalCost          float64   `json:"total_cost"`
}

// SessionUsageTurn is one interaction (a user message and the agent's work on it).
type SessionUsageTurn struct {
	InteractionID    string    `json:"interaction_id"`
	Prompt           string    `json:"prompt"`
	Started          time.Time `json:"started"`
	Completed        time.Time `json:"completed,omitempty"`
	State            string    `json:"state"`
	Calls            int       `json:"calls"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens"`
	CacheHitRatio    *float64  `json:"cache_hit_ratio"`
	LLMMs            int64     `json:"llm_ms"`
	TotalCost        float64   `json:"total_cost"`
}
