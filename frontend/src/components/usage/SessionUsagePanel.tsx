// SessionUsagePanel shows what one session costs and how fast it runs: spend,
// tokens, LLM latency and prompt-cache hits, overall, per LLM call and per
// turn. Compact enough for the session workspace's side panel; the charts are
// the org usage page's ShadcnAreaChart with the same series colours.

import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import { SessionUsage, useSessionUsage } from '../../services/sessionUsageService'
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart'
import SessionResourcesPanel from './SessionResourcesPanel'
import { CompactTable, OverviewStat, OverviewStatStrip, UsageChartGrid, UsageGroup } from './UsageOverview'

// Same palette as the usage page (TotalCost / TokenUsage).
const TOKEN_SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Input', color: '#3b82f6' },
  { key: 'cacheRead', label: 'Cache read', color: '#22c55e' },
  { key: 'output', label: 'Output', color: '#a855f7' },
]
const LATENCY_SERIES: ShadcnSeries[] = [
  { key: 'duration', label: 'Call', color: '#3b82f6' },
  { key: 'ttft', label: 'First token', color: '#f59e0b' },
]
const CACHE_SERIES: ShadcnSeries[] = [{ key: 'hit', label: 'Cache hit', color: '#22c55e' }]

export const formatTokens = (n: number): string => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return `${Math.round(n)}`
}

export const formatCost = (n: number): string => (n >= 1 ? `$${n.toFixed(2)}` : `$${n.toFixed(4)}`)

export const formatSeconds = (ms: number): string => {
  const s = ms / 1000
  if (s >= 60) return `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`
  return `${s < 10 ? s.toFixed(1) : Math.round(s)}s`
}

const formatPercent = (r: number | null | undefined): string => (r == null ? '–' : `${Math.round(r * 100)}%`)

// Chart rows per LLM call, keyed by the call's timestamp.
export const buildUsageRows = (usage: SessionUsage) => {
  const tokens = usage.calls.map(c => ({
    date: c.created,
    input: Math.max(c.prompt_tokens - c.cache_read_tokens, 0),
    cacheRead: c.cache_read_tokens,
    output: c.completion_tokens,
  }))
  const latency = usage.calls.map(c => ({
    date: c.created,
    duration: c.duration_ms / 1000,
    ttft: c.time_to_first_token_ms / 1000,
  }))
  const cache = usage.calls
    .filter(c => c.prompt_tokens > 0)
    .map(c => ({ date: c.created, hit: c.cache_read_tokens / c.prompt_tokens }))
  return { tokens, latency, cache }
}

export const SessionUsageView: FC<{ usage: SessionUsage }> = ({ usage }) => {
  const s = usage.summary
  const rows = useMemo(() => buildUsageRows(usage), [usage])
  // Self-hosted providers often have no price: say so rather than show $0.
  const priced = s.total_cost > 0
  const totalTokens = s.prompt_tokens + s.completion_tokens

  if (s.calls === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No LLM calls recorded for this session yet.
      </Typography>
    )
  }

  return (
    <Stack spacing={2}>
      <OverviewStatStrip columns={4} fit="container">
        <OverviewStat emphasis label="Spend" value={priced ? formatCost(s.total_cost) : 'n/a'} detail={priced ? `${s.calls} LLM calls` : 'No price for this model'} />
        <OverviewStat emphasis label="Tokens" value={formatTokens(totalTokens)} detail={`${formatTokens(s.prompt_tokens)} in · ${formatTokens(s.completion_tokens)} out`} />
        <OverviewStat emphasis label="Cache hit" value={formatPercent(s.cache_hit_ratio)} detail={`${formatTokens(s.cache_read_tokens)} tokens cached`} />
        <OverviewStat emphasis label="LLM time" value={formatSeconds(s.llm_ms)} detail={`First token p50 ${formatSeconds(s.ttft_p50_ms)}`} />
      </OverviewStatStrip>

      <ShadcnAreaChart
        title="Tokens per call"
        headline={`${formatTokens(totalTokens)} across ${s.calls} calls`}
        data={rows.tokens}
        series={TOKEN_SERIES}
        valueFormatter={formatTokens}
        cardHeight="auto"
        chartHeight={150}
        yAxisWidth={48}
        showTime
      />
      <UsageChartGrid fit="container">
        <ShadcnAreaChart
          title="Latency per call"
          headline={`p50 ${formatSeconds(s.duration_p50_ms)} · p90 ${formatSeconds(s.duration_p90_ms)}`}
          data={rows.latency}
          series={LATENCY_SERIES}
          valueFormatter={v => `${v.toFixed(1)}s`}
          stacked={false}
          variant="line"
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={48}
          showTime
        />
        <ShadcnAreaChart
          title="Cache hit ratio"
          headline={`${formatPercent(s.cache_hit_ratio)} of input tokens`}
          data={rows.cache}
          series={CACHE_SERIES}
          valueFormatter={v => `${Math.round(v * 100)}%`}
          stacked={false}
          variant="line"
          yDomain={[0, 1]}
          hideLegend
          zeroIsData
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={48}
          showTime
        />
      </UsageChartGrid>

      {usage.turns.length > 0 && (
        <CompactTable
          title="Per turn"
          columns={[
            { key: 'message', label: 'Message' },
            { key: 'calls', label: 'Calls' },
            { key: 'tokens', label: 'Tokens' },
            { key: 'cache', label: 'Cache' },
            { key: 'llm', label: 'LLM time' },
            ...(priced ? [{ key: 'cost', label: 'Cost' }] : []),
          ]}
          rows={usage.turns.map(t => ({
            id: t.interaction_id,
            title: t.prompt,
            cells: {
              message: t.prompt || '—',
              calls: t.calls,
              tokens: formatTokens(t.prompt_tokens + t.completion_tokens),
              cache: formatPercent(t.cache_hit_ratio),
              llm: formatSeconds(t.llm_ms),
              cost: formatCost(t.total_cost),
            },
          }))}
          footer={(
            <Typography variant="caption" color="text.secondary">
              {s.models.join(', ')} · first token p90 {formatSeconds(s.ttft_p90_ms)}
              {usage.truncated ? ' · showing the most recent calls only' : ''}
            </Typography>
          )}
        />
      )}
    </Stack>
  )
}

// callSpan says when the session's LLM calls happened, so their charts and the
// sandbox charts (which have their own window) are never read as one timeline.
export const callSpan = (usage?: SessionUsage | null): string => {
  const calls = usage?.calls ?? []
  if (!calls.length) return 'Spend, tokens, latency and prompt-cache hits of this session\'s LLM calls.'
  const first = new Date(calls[0].created)
  const last = new Date(calls[calls.length - 1].created)
  const day: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }
  const time: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' }
  const sameDay = first.toDateString() === last.toDateString()
  const span = sameDay
    ? `${first.toLocaleDateString(undefined, day)}, ${first.toLocaleTimeString(undefined, time)} – ${last.toLocaleTimeString(undefined, time)}`
    : `${first.toLocaleDateString(undefined, day)} – ${last.toLocaleDateString(undefined, day)}`
  return `All ${usage?.summary.calls ?? calls.length} LLM calls of this session, ${span}.`
}

const SessionLLMUsage: FC<{ sessionId: string }> = ({ sessionId }) => {
  const { data, isLoading, error } = useSessionUsage(sessionId, { refetchInterval: 15000 })
  return (
    <UsageGroup title="LLM usage" description={callSpan(data)}>
      {isLoading ? (
        <CircularProgress size={18} />
      ) : error || !data ? (
        <Typography variant="body2" color="text.secondary">
          Usage is not available for this session.
        </Typography>
      ) : (
        <SessionUsageView usage={data} />
      )}
    </UsageGroup>
  )
}

// The panel sizes its grids by its own width: it lives in a side panel as
// often as on a full page.
const SessionUsagePanel: FC<{ sessionId: string }> = ({ sessionId }) => (
  <Box sx={{ containerType: 'inline-size' }}>
    <Stack spacing={4}>
      <SessionLLMUsage sessionId={sessionId} />
      <SessionResourcesPanel sessionId={sessionId} />
    </Stack>
  </Box>
)

export default SessionUsagePanel
