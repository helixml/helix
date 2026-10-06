// SessionUsagePanel shows what one session costs and how fast it runs: spend,
// tokens, LLM latency and prompt-cache hits, overall, per LLM call and per
// turn. Compact enough for the session workspace's side panel; the charts are
// the org usage page's ShadcnAreaChart with the same series colours.

import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import useLightTheme from '../../hooks/useLightTheme'
import { SessionUsage, useSessionUsage } from '../../services/sessionUsageService'
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart'

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

const Tile: FC<{ label: string; value: string; note?: string }> = ({ label, value, note }) => {
  const lightTheme = useLightTheme()
  return (
    <Box
      sx={{
        p: 1.25,
        borderRadius: 1.5,
        border: '1px solid',
        borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.06)',
        background: lightTheme.isLight ? 'rgba(0,0,0,0.015)' : 'rgba(255,255,255,0.02)',
        minWidth: 0,
      }}
    >
      <Typography variant="caption" sx={{ fontSize: '0.65rem', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'text.secondary' }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontSize: '1rem', fontWeight: 600, fontVariantNumeric: 'tabular-nums', lineHeight: 1.3 }} noWrap>
        {value}
      </Typography>
      {note && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontVariantNumeric: 'tabular-nums' }} noWrap>
          {note}
        </Typography>
      )}
    </Box>
  )
}

export const SessionUsageView: FC<{ usage: SessionUsage }> = ({ usage }) => {
  const lightTheme = useLightTheme()
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
    <Stack spacing={1.5}>
      <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))', gap: 1 }}>
        <Tile label="Spend" value={priced ? formatCost(s.total_cost) : 'n/a'} note={priced ? `${s.calls} LLM calls` : 'no price for this model'} />
        <Tile label="Tokens" value={formatTokens(totalTokens)} note={`${formatTokens(s.prompt_tokens)} in · ${formatTokens(s.completion_tokens)} out`} />
        <Tile label="Cache hit" value={formatPercent(s.cache_hit_ratio)} note={`${formatTokens(s.cache_read_tokens)} cached`} />
        <Tile label="LLM time" value={formatSeconds(s.llm_ms)} note={`first token p50 ${formatSeconds(s.ttft_p50_ms)}`} />
      </Box>

      <ShadcnAreaChart
        title="Tokens per call"
        headline={formatTokens(totalTokens)}
        data={rows.tokens}
        series={TOKEN_SERIES}
        valueFormatter={formatTokens}
        cardHeight="auto"
        chartHeight={130}
        yAxisWidth={48}
        showTime
      />
      <ShadcnAreaChart
        title="Latency per call"
        headline={`p50 ${formatSeconds(s.duration_p50_ms)} · p90 ${formatSeconds(s.duration_p90_ms)}`}
        data={rows.latency}
        series={LATENCY_SERIES}
        valueFormatter={v => `${v.toFixed(1)}s`}
        stacked={false}
        variant="line"
        cardHeight="auto"
        chartHeight={120}
        yAxisWidth={48}
        showTime
      />
      <ShadcnAreaChart
        title="Cache hit ratio"
        headline={formatPercent(s.cache_hit_ratio)}
        data={rows.cache}
        series={CACHE_SERIES}
        valueFormatter={v => `${Math.round(v * 100)}%`}
        stacked={false}
        variant="line"
        yDomain={[0, 1]}
        hideLegend
        zeroIsData
        cardHeight="auto"
        chartHeight={100}
        yAxisWidth={48}
        showTime
      />

      {usage.turns.length > 0 && (
        <Box>
          <Typography variant="caption" sx={{ fontSize: '0.65rem', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'text.secondary' }}>
            Per turn
          </Typography>
          <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse', mt: 0.5, fontSize: '0.78rem', fontVariantNumeric: 'tabular-nums' }}>
            <Box component="thead">
              <Box component="tr" sx={{ color: 'text.secondary', textAlign: 'left' }}>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5 }}>Message</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Calls</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Tokens</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Cache</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>LLM</Box>
                {priced && <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Cost</Box>}
              </Box>
            </Box>
            <Box component="tbody">
              {usage.turns.map(t => (
                <Box
                  component="tr"
                  key={t.interaction_id}
                  sx={{ borderTop: '1px solid', borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(255,255,255,0.06)' }}
                >
                  <Box component="td" sx={{ py: 0.5, pr: 1, maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={t.prompt}>
                    {t.prompt || '—'}
                  </Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{t.calls}</Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{formatTokens(t.prompt_tokens + t.completion_tokens)}</Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{formatPercent(t.cache_hit_ratio)}</Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{formatSeconds(t.llm_ms)}</Box>
                  {priced && <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{formatCost(t.total_cost)}</Box>}
                </Box>
              ))}
            </Box>
          </Box>
        </Box>
      )}

      <Typography variant="caption" color="text.secondary">
        {s.models.join(', ')} · first token p90 {formatSeconds(s.ttft_p90_ms)}
        {usage.truncated ? ' · showing the most recent calls only' : ''}
      </Typography>
    </Stack>
  )
}

const SessionUsagePanel: FC<{ sessionId: string }> = ({ sessionId }) => {
  const { data, isLoading, error } = useSessionUsage(sessionId, { refetchInterval: 15000 })
  if (isLoading) return <CircularProgress size={18} />
  if (error || !data) {
    return (
      <Typography variant="body2" color="text.secondary">
        Usage is not available for this session.
      </Typography>
    )
  }
  return <SessionUsageView usage={data} />
}

export default SessionUsagePanel
