import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'

// GET /api/v1/sessions/{id}/usage — LLM spend, tokens, latency and prompt-cache
// hits for one session (types.SessionUsage in api/pkg/types/session_usage.go).
// The generated TypesSessionUsage marks every field optional; the API always
// sends them, so the stricter shapes below are what the panel works with.

export interface SessionUsageSummary {
  calls: number
  prompt_tokens: number
  completion_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cache_hit_ratio: number | null
  total_cost: number
  llm_ms: number
  ttft_p50_ms: number
  ttft_p90_ms: number
  duration_p50_ms: number
  duration_p90_ms: number
  models: string[]
}

export interface SessionUsageCall {
  created: string
  interaction_id?: string
  model: string
  duration_ms: number
  time_to_first_token_ms: number
  prompt_tokens: number
  completion_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  total_cost: number
}

export interface SessionUsageTurn {
  interaction_id: string
  prompt: string
  started: string
  completed?: string
  state: string
  calls: number
  prompt_tokens: number
  completion_tokens: number
  cache_read_tokens: number
  cache_hit_ratio: number | null
  llm_ms: number
  total_cost: number
}

export interface SessionUsage {
  session_id: string
  summary: SessionUsageSummary
  calls: SessionUsageCall[]
  turns: SessionUsageTurn[]
  truncated?: boolean
}

export const sessionUsageQueryKey = (sessionId: string) => ['session-usage', sessionId]

export function useSessionUsage(sessionId: string | undefined, options?: { refetchInterval?: number | false }) {
  const api = useApi()
  return useQuery({
    queryKey: sessionUsageQueryKey(sessionId ?? ''),
    queryFn: async () => {
      if (!sessionId) return null
      const res = await api.getApiClient().v1SessionsUsageDetail(sessionId)
      return res.data as SessionUsage
    },
    enabled: !!sessionId,
    refetchInterval: options?.refetchInterval,
  })
}
