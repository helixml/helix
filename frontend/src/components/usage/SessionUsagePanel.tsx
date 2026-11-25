// SessionUsagePanel shows what one session costs and how fast it runs: spend,
// tokens, LLM latency and prompt-cache hits, overall, per LLM call and per
// turn. Compact enough for the session workspace's side panel; the charts are
// the org usage page's ShadcnAreaChart with the same series colours.

import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import { SessionUsage, useSessionUsage } from '../../services/sessionUsageService'
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart'
import SessionResourcesPanel from './SessionResourcesPanel'
import { CompactTable, OverviewStat, OverviewStatStrip, SegmentBar, SegmentBarPanel, UsageChartGrid, UsageGroup, UsageSwitch } from './UsageOverview'

// Same palette as the usage page (TotalCost / TokenUsage).
const TOKEN_SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Uncached input', color: '#3b82f6' },
  { key: 'cacheRead', label: 'Cache read', color: '#22c55e' },
  { key: 'output', label: 'Output', color: '#a855f7' },
]
const CALL_SERIES: ShadcnSeries[] = [{ key: 'calls', label: 'LLM calls', color: '#3b82f6' }]
const COST_SERIES: ShadcnSeries[] = [{ key: 'cost', label: 'Cost', color: '#3b82f6' }]
const LATENCY_SERIES: ShadcnSeries[] = [
  { key: 'duration', label: 'Call', color: '#2563eb' },
  { key: 'ttft', label: 'First token', color: '#7cb4fa' },
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
  return { tokens, latency }
}

type ActivityMetric = 'tokens' | 'calls' | 'cost' | 'per-call'

const MINUTE = 60_000
const BUCKETS = [1, 5, 15, 30, 60, 180, 360, 720, 1440].map(m => m * MINUTE)

// bucketCalls sums the session's calls into even time buckets, sized so the
// span fits in about 40 points: a ten-minute run reads per minute, a week-long
// task per day. Empty buckets stay, so idle time shows as a gap.
export interface CallBucket {
  date: string
  input: number
  cacheRead: number
  output: number
  calls: number
  cost: number
  prompt: number
}

export const bucketCalls = (calls: SessionUsage['calls']): { size: number; rows: CallBucket[]; cache: Array<{ date: string; hit: number }> } => {
  if (!calls.length) return { size: MINUTE, rows: [], cache: [] }
  const times = calls.map(c => new Date(c.created).getTime())
  const first = Math.min(...times)
  const last = Math.max(...times)
  const size = BUCKETS.find(b => (last - first) / b <= 40) ?? BUCKETS[BUCKETS.length - 1]
  const start = Math.floor(first / size) * size
  const count = Math.floor((last - start) / size) + 1
  const rows: CallBucket[] = Array.from({ length: count }, (_, i) => ({
    date: new Date(start + i * size).toISOString(),
    input: 0, cacheRead: 0, output: 0, calls: 0, cost: 0, prompt: 0,
  }))
  calls.forEach((c, i) => {
    const row = rows[Math.floor((times[i] - start) / size)]
    row.input += Math.max(c.prompt_tokens - c.cache_read_tokens, 0)
    row.cacheRead += c.cache_read_tokens
    row.output += c.completion_tokens
    row.calls += 1
    row.cost += c.total_cost
    row.prompt += c.prompt_tokens
  })
  // Cache hit per bucket: buckets without input stay empty, not 0%.
  const cache = rows.filter(r => r.prompt > 0).map(r => ({ date: r.date, hit: r.cacheRead / r.prompt }))
  return { size, rows, cache }
}

const bucketLabel = (size: number): string => {
  if (size >= 1440 * MINUTE) return 'per day'
  if (size >= 60 * MINUTE) return `per ${size / (60 * MINUTE)} h`
  return size === MINUTE ? 'per minute' : `per ${size / MINUTE} min`
}

export const SessionUsageView: FC<{ usage: SessionUsage }> = ({ usage }) => {
  const s = usage.summary
  const rows = useMemo(() => buildUsageRows(usage), [usage])
  const buckets = useMemo(() => bucketCalls(usage.calls), [usage])
  const [metric, setMetric] = useState<ActivityMetric>('tokens')
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

      <SegmentBarPanel fit="container" single>
        <SegmentBar
        label="Tokens by type"
        segments={[
          { label: 'Uncached input', value: Math.max(s.prompt_tokens - s.cache_read_tokens, 0), color: TOKEN_SERIES[0].color },
          { label: 'Cache read', value: s.cache_read_tokens, color: TOKEN_SERIES[1].color },
          { label: 'Output', value: s.completion_tokens, color: TOKEN_SERIES[2].color },
        ]}
        format={formatTokens}
        />
      </SegmentBarPanel>

      <ShadcnAreaChart
        title={metric === 'per-call' ? 'Tokens per call' : `${{ tokens: 'Tokens', calls: 'LLM calls', cost: 'Spend' }[metric]} ${bucketLabel(buckets.size)}`}
        headline={{
          tokens: `${formatTokens(totalTokens)} across ${s.calls} calls`,
          calls: `${s.calls} calls`,
          cost: `${formatCost(s.total_cost)} across ${s.calls} calls`,
          'per-call': `${formatTokens(s.calls ? totalTokens / s.calls : 0)} per call on average`,
        }[metric]}
        action={(
          <UsageSwitch<ActivityMetric>
            label="Metric"
            value={metric}
            options={[
              { value: 'tokens', label: 'Tokens' },
              { value: 'calls', label: 'Calls' },
              ...(priced ? [{ value: 'cost' as const, label: 'Cost' }] : []),
              { value: 'per-call', label: 'Per call' },
            ]}
            onChange={setMetric}
          />
        )}
        data={metric === 'per-call' ? rows.tokens : buckets.rows}
        series={metric === 'calls' ? CALL_SERIES : metric === 'cost' ? COST_SERIES : TOKEN_SERIES}
        valueFormatter={metric === 'calls' ? (v => `${Math.round(v)}`) : metric === 'cost' ? formatCost : formatTokens}
        variant={metric === 'per-call' ? 'area' : 'trend'}
        hideLegend={metric === 'calls' || metric === 'cost'}
        cardHeight="auto"
        chartHeight={170}
        yAxisWidth={48}
        showTime={metric === 'per-call' || buckets.size < 1440 * MINUTE}
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
          title={`Cache hit ratio ${bucketLabel(buckets.size)}`}
          headline={`${formatPercent(s.cache_hit_ratio)} of input tokens`}
          data={buckets.cache}
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
            muted: !t.calls,
            cells: {
              message: t.prompt || '—',
              // A turn with no LLM calls (a notice, an agent switch) shows dashes, not zeros.
              calls: t.calls || '–',
              tokens: t.calls ? formatTokens(t.prompt_tokens + t.completion_tokens) : '–',
              cache: formatPercent(t.cache_hit_ratio),
              llm: t.calls ? formatSeconds(t.llm_ms) : '–',
              cost: t.calls ? formatCost(t.total_cost) : '–',
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
