import React, { FC, ReactNode, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { alpha } from '@mui/material/styles'
import { ChevronRight } from 'lucide-react'

import { TypesUsageProviderKind } from '../../api/api'
import type { TypesOrgUsageSummaryResponse, TypesUsageBreakdownRow } from '../../api/api'
import { formatCompact, formatCost, formatHours, formatNumber, formatPercent } from '../../utils/usageFormat'
import { getTotalInputTokens } from '../../utils/usageMetrics'
import { SegmentBar, UsageSegment } from './UsageOverview'
import { PROVIDER_KIND_COLOR, PROVIDER_KIND_GROUP_LABEL, PROVIDER_KIND_ORDER, UsageProviderIcon, hasProviderBrand } from './usageProviders'
import UsageLimits from './UsageLimits'

export type UsageSummaryView = 'cost' | 'tokens' | 'limits'
export type UsageCostMode = 'billed' | 'usage' | 'unpriced'

type Metric = 'cost' | 'tokens'

// Token types share hues with the token charts in the LLM tokens section.
const TYPE_COLORS = { input: '#2563eb', cacheRead: '#16a34a', cacheWrite: '#d97706', output: '#9333ea' }

const MODEL_LIMIT = 8

const metricOf = (row: { total_cost?: number; total_tokens?: number }, metric: Metric) => (
  metric === 'cost' ? row.total_cost ?? 0 : row.total_tokens ?? 0
)

const formatMetric = (value: number, metric: Metric) => (metric === 'cost' ? formatCost(value) : formatCompact(value))

const dayKey = (value?: string) => (value || '').slice(0, 10)

// enumerateDays lists every day in the range, so days with no activity keep
// their slot on the chart instead of closing the gap.
export const enumerateDays = (from: string, to: string): string[] => {
  const days: string[] = []
  const start = new Date(`${from}T00:00:00Z`)
  const end = new Date(`${to}T00:00:00Z`)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return days
  for (let day = start; day <= end && days.length < 400; day = new Date(day.getTime() + 86_400_000)) {
    days.push(day.toISOString().slice(0, 10))
  }
  return days
}

const formatAxisDay = (day: string) => new Date(`${day}T00:00:00Z`)
  .toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })

export interface UsageDailyBar {
  day: string
  total: number
  values: Array<{ key: string; value: number }>
}

// buildDailyBars stacks each provider's value per day, in the providers' order.
export const buildDailyBars = (
  days: string[],
  series: Array<{ key: string; metrics?: Array<{ date?: string; total_cost?: number; total_tokens?: number }> }>,
  metric: Metric,
): UsageDailyBar[] => {
  const byProvider = series.map(item => {
    const values = new Map<string, number>()
    for (const point of item.metrics || []) {
      const key = dayKey(point.date)
      values.set(key, (values.get(key) ?? 0) + metricOf(point, metric))
    }
    return { key: item.key, values }
  })
  return days.map(day => {
    const values = byProvider.map(item => ({ key: item.key, value: item.values.get(day) ?? 0 }))
    return { day, values, total: values.reduce((sum, entry) => sum + entry.value, 0) }
  })
}

export interface UsageProviderGroup {
  kind: TypesUsageProviderKind
  total: number
  rows: TypesUsageBreakdownRow[]
}

// groupProviders puts providers under who they were paid through, largest group first.
export const groupProviders = (rows: TypesUsageBreakdownRow[], metric: Metric): UsageProviderGroup[] => {
  const groups = new Map<TypesUsageProviderKind, UsageProviderGroup>()
  for (const row of rows) {
    const kind = row.provider_kind || TypesUsageProviderKind.UsageProviderKindUnknown
    const group = groups.get(kind) ?? { kind, total: 0, rows: [] }
    group.total += metricOf(row, metric)
    group.rows.push(row)
    groups.set(kind, group)
  }
  return Array.from(groups.values())
    .map(group => ({ ...group, rows: [...group.rows].sort((a, b) => metricOf(b, metric) - metricOf(a, metric)) }))
    .sort((a, b) => b.total - a.total || PROVIDER_KIND_ORDER.indexOf(a.kind) - PROVIDER_KIND_ORDER.indexOf(b.kind))
}

export const SegmentedTabs = <T extends string,>({ label, value, options, onChange, compact }: {
  label: string
  value: T
  options: Array<{ value: T; label: string; disabled?: boolean }>
  onChange: (value: T) => void
  compact?: boolean
}) => (
  <Box
    role="tablist"
    aria-label={label}
    sx={{
      display: 'grid',
      gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))`,
      p: 0.5,
      gap: 0.5,
      borderRadius: 999,
      bgcolor: theme => alpha(theme.palette.text.primary, 0.06),
    }}
  >
    {options.map(option => {
      const selected = option.value === value
      return (
        <Box
          key={option.value}
          component="button"
          type="button"
          role="tab"
          aria-selected={selected}
          disabled={option.disabled}
          onClick={() => onChange(option.value)}
          sx={{
            all: 'unset',
            boxSizing: 'border-box',
            textAlign: 'center',
            cursor: option.disabled ? 'default' : 'pointer',
            borderRadius: 999,
            py: compact ? 0.75 : 1,
            px: 1.5,
            fontSize: compact ? '0.8125rem' : '0.875rem',
            fontWeight: selected ? 600 : 500,
            color: option.disabled ? 'text.disabled' : selected ? 'text.primary' : 'text.secondary',
            bgcolor: theme => selected ? alpha(theme.palette.text.primary, 0.12) : 'transparent',
            transition: 'background-color 150ms, color 150ms',
            '&:hover': { color: option.disabled ? undefined : 'text.primary' },
            '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: 1 },
          }}
        >
          {option.label}
        </Box>
      )
    })}
  </Box>
)

export const SummaryCard: FC<{ children: ReactNode; sx?: object }> = ({ children, sx }) => (
  <Box
    sx={{
      bgcolor: 'background.paper',
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 3,
      overflow: 'hidden',
      minWidth: 0,
      ...sx,
    }}
  >
    {children}
  </Box>
)

export const SummarySection: FC<{ title: string; action?: ReactNode; children: ReactNode }> = ({ title, action, children }) => (
  <Box sx={{ minWidth: 0 }}>
    <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 0.5, mb: 1, minHeight: 28 }}>
      <Typography variant="body2" component="h3" sx={{ fontWeight: 600, color: 'text.secondary' }}>{title}</Typography>
      {action}
    </Stack>
    {children}
  </Box>
)

const Dot: FC<{ color: string; size?: number }> = ({ color, size = 8 }) => (
  <Box aria-hidden sx={{ width: size, height: size, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />
)

const DailyBars: FC<{
  bars: UsageDailyBar[]
  colors: Record<string, string>
  names: Record<string, string>
  metric: Metric
}> = ({ bars, colors, names, metric }) => {
  const max = bars.reduce((peak, bar) => Math.max(peak, bar.total), 0)
  return (
    <Box
      role="img"
      aria-label={`Daily ${metric === 'cost' ? 'cost' : 'processed tokens'} by provider`}
      sx={{ height: { xs: 180, md: 240 }, display: 'flex', alignItems: 'flex-end', gap: bars.length > 60 ? '1px' : '3px' }}
    >
      {bars.map(bar => {
        const shown = bar.values.filter(entry => entry.value > 0)
        return (
          <Tooltip
            key={bar.day}
            placement="top"
            title={(
              <Stack spacing={0.5} sx={{ py: 0.25 }}>
                <Typography variant="caption" sx={{ fontWeight: 600 }}>
                  {formatAxisDay(bar.day)} · {formatMetric(bar.total, metric)}
                </Typography>
                {shown.slice().sort((a, b) => b.value - a.value).map(entry => (
                  <Stack key={entry.key} direction="row" alignItems="center" spacing={0.75}>
                    <Dot color={colors[entry.key]} />
                    <Typography variant="caption" sx={{ flex: 1 }}>{names[entry.key] || entry.key}</Typography>
                    <Typography variant="caption" sx={{ fontVariantNumeric: 'tabular-nums' }}>{formatMetric(entry.value, metric)}</Typography>
                  </Stack>
                ))}
              </Stack>
            )}
          >
            <Box sx={{ flex: 1, minWidth: 0, height: '100%', display: 'flex', alignItems: 'flex-end' }}>
              <Box
                sx={{
                  width: '100%',
                  height: max === 0 ? 0 : `${(bar.total / max) * 100}%`,
                  minHeight: bar.total > 0 ? 2 : 0,
                  display: 'flex',
                  flexDirection: 'column-reverse',
                  borderRadius: '3px 3px 0 0',
                  overflow: 'hidden',
                }}
              >
                {shown.map(entry => (
                  <Box key={entry.key} sx={{ flex: `${entry.value} 0 0`, bgcolor: colors[entry.key] }} />
                ))}
              </Box>
            </Box>
          </Tooltip>
        )
      })}
    </Box>
  )
}

const HeroCard: FC<{
  label: string
  value: string
  detail: string
  bars: UsageDailyBar[]
  providers: TypesUsageBreakdownRow[]
  colors: Record<string, string>
  metric: Metric
}> = ({ label, value, detail, bars, providers, colors, metric }) => {
  const names = Object.fromEntries(providers.map(row => [row.provider || 'unknown', row.name || row.provider || 'Unknown']))
  const hasActivity = bars.some(bar => bar.total > 0)
  return (
    <SummaryCard sx={{ p: { xs: 2, sm: 2.5 }, display: 'flex', flexDirection: 'column', gap: 2 }}>
      <Box>
        <Typography variant="body2" color="text.secondary">{label}</Typography>
        <Typography
          component="p"
          sx={{ fontSize: { xs: '2.25rem', sm: '2.5rem' }, fontWeight: 700, lineHeight: 1.15, letterSpacing: '-0.02em', fontVariantNumeric: 'tabular-nums' }}
        >
          {value}
        </Typography>
        <Typography variant="body2" color="text.secondary">{detail}</Typography>
      </Box>
      <Box sx={{ minHeight: { xs: 180, md: 240 }, display: 'flex', alignItems: 'flex-end' }}>
        {hasActivity ? (
          <Box sx={{ width: '100%' }}>
            <DailyBars bars={bars} colors={colors} names={names} metric={metric} />
          </Box>
        ) : (
          <Typography variant="body2" color="text.secondary" sx={{ width: '100%', textAlign: 'center', alignSelf: 'center' }}>
            No activity in this range.
          </Typography>
        )}
      </Box>
      <Stack spacing={1.25}>
        <Stack direction="row" justifyContent="space-between">
          <Typography variant="caption" color="text.secondary">{bars.length ? formatAxisDay(bars[0].day) : ''}</Typography>
          <Typography variant="caption" color="text.secondary">{bars.length ? formatAxisDay(bars[bars.length - 1].day) : ''}</Typography>
        </Stack>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 2, rowGap: 0.75 }}>
          {providers.map(row => (
            <Stack key={row.provider} direction="row" alignItems="center" spacing={0.75} sx={{ minWidth: 0 }}>
              <Dot color={colors[row.provider || 'unknown']} />
              <Typography variant="caption" color="text.secondary" noWrap sx={{ maxWidth: 200 }}>{row.name || row.provider}</Typography>
            </Stack>
          ))}
        </Box>
      </Stack>
    </SummaryCard>
  )
}

// Beside the chart on wide screens the printed share is enough; the track is for the phone list.
const ShareTrack: FC<{ share: number; color: string }> = ({ share, color }) => (
  <Box sx={{ display: { lg: 'none' }, height: 4, borderRadius: 999, bgcolor: theme => alpha(theme.palette.text.primary, 0.08), overflow: 'hidden' }}>
    <Box sx={{ width: `${Math.min(Math.max(share, 0), 1) * 100}%`, height: '100%', borderRadius: 999, bgcolor: color }} />
  </Box>
)

const ProvidersCard: FC<{
  groups: UsageProviderGroup[]
  colors: Record<string, string>
  metric: Metric
  total: number
}> = ({ groups, colors, metric, total }) => (
  <SummaryCard>
    {groups.map((group, groupIndex) => (
      <Box key={group.kind} sx={{ borderTop: groupIndex > 0 ? '1px solid' : 'none', borderColor: 'divider' }}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="baseline"
          sx={{ px: 2, pt: 1.5, pb: 0.25 }}
        >
          <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
            {PROVIDER_KIND_GROUP_LABEL[group.kind]}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
            {formatMetric(group.total, metric)} · {formatPercent(total > 0 ? group.total / total : 0)}
          </Typography>
        </Stack>
        {group.rows.map(row => {
          const value = metricOf(row, metric)
          const share = total > 0 ? value / total : 0
          const key = row.provider || 'unknown'
          // No known rates is not the same as free: $0.00 is kept for a real zero.
          const unpriced = metric === 'cost' && (row.total_cost ?? 0) === 0 && (row.total_tokens ?? 0) > 0
          return (
            <Stack key={key} spacing={0.75} sx={{ px: 2, py: { xs: 1.25, lg: 1 } }}>
              <Stack direction="row" alignItems="center" spacing={1.25} sx={{ minWidth: 0 }}>
                <Dot color={colors[key]} size={10} />
                {hasProviderBrand(row.provider, row.name) && (
                  <Box sx={{ display: 'flex', color: 'text.secondary', flexShrink: 0 }}>
                    <UsageProviderIcon provider={row.provider} name={row.name} size={16} />
                  </Box>
                )}
                <Typography variant="body1" noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 500 }} title={row.name}>
                  {row.name || row.provider || 'Unknown'}
                </Typography>
                <Typography variant="body1" color={unpriced ? 'text.secondary' : undefined} sx={{ fontVariantNumeric: 'tabular-nums', fontWeight: 500 }}>
                  {unpriced ? 'Unpriced' : formatMetric(value, metric)}
                </Typography>
              </Stack>
              <ShareTrack share={share} color={colors[key]} />
              <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                {unpriced
                  ? `No known rates · ${formatCompact(row.total_tokens)} tokens`
                  : metric === 'cost'
                    ? `${formatPercent(share)} of cost · ${formatCompact(row.total_tokens)} tokens`
                    : `${formatPercent(share)} of tokens · ${(row.total_cost ?? 0) > 0 ? formatCost(row.total_cost) : 'unpriced'}`}
              </Typography>
            </Stack>
          )
        })}
      </Box>
    ))}
  </SummaryCard>
)

export const MetricCell: FC<{ label: string; value: string; detail: string }> = ({ label, value, detail }) => (
  <Box sx={{ p: 2, minWidth: 0 }}>
    <Typography variant="body2" color="text.secondary" noWrap>{label}</Typography>
    <Typography sx={{ fontSize: '1.25rem', fontWeight: 600, lineHeight: 1.4, fontVariantNumeric: 'tabular-nums' }} noWrap>{value}</Typography>
    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.4 }}>{detail}</Typography>
  </Box>
)

const ModelsCard: FC<{ models: TypesUsageBreakdownRow[]; metric: Metric; total: number }> = ({ models, metric, total }) => (
  <SummaryCard>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: 'repeat(2, minmax(0, 1fr))' } }}>
      {models.map((row, index) => {
        const value = metricOf(row, metric)
        const share = total > 0 ? value / total : 0
        const priced = (row.total_cost ?? 0) > 0
        return (
          <Stack
            key={row.id || `${row.provider}:${row.model}`}
            direction="row"
            alignItems="center"
            spacing={1.5}
            sx={{
              px: 2,
              py: 1.5,
              minWidth: 0,
              borderTop: { xs: index > 0 ? '1px solid' : 'none', md: index > 1 ? '1px solid' : 'none' },
              borderLeft: { md: index % 2 === 1 ? '1px solid' : 'none' },
              borderColor: { xs: 'divider', md: 'divider' },
            }}
          >
            <Box sx={{ display: 'flex', color: 'text.secondary', flexShrink: 0 }}>
              <UsageProviderIcon provider={row.provider} size={18} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" noWrap sx={{ fontWeight: 500 }} title={row.model || row.name}>{row.model || row.name || 'Unknown'}</Typography>
              <Typography variant="caption" color="text.secondary" noWrap sx={{ display: 'block' }}>
                {metric === 'cost'
                  ? priced ? `${formatPercent(share)} of cost · ${formatCompact(row.total_tokens)} tokens` : `No known rates · ${formatCompact(row.total_tokens)} tokens`
                  : `${formatPercent(share)} of tokens · ${priced ? formatCost(row.total_cost) : 'unpriced'}`}
              </Typography>
            </Box>
            <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', fontWeight: 500 }}>
              {metric === 'cost' && !priced ? 'Unpriced' : formatMetric(value, metric)}
            </Typography>
          </Stack>
        )
      })}
    </Box>
  </SummaryCard>
)

export interface UsageSummaryProps {
  orgId: string
  data?: TypesOrgUsageSummaryResponse
  from: string
  to: string
  costMode: UsageCostMode
  providerColors: Record<string, string>
  view: UsageSummaryView
  onViewChange: (view: UsageSummaryView) => void
  onShowAllModels: () => void
}

// UsageSummary is the org usage overview: one headline number over a daily
// chart split by provider, the providers grouped by who they were paid
// through, totals, and the models behind them. Limits sits beside it as a tab.
const UsageSummary: FC<UsageSummaryProps> = ({
  orgId,
  data,
  from,
  to,
  costMode,
  providerColors,
  view,
  onViewChange,
  onShowAllModels,
}) => {
  const metric: Metric = view === 'cost' ? 'cost' : 'tokens'
  const [showAllModels, setShowAllModels] = useState(false)

  const metrics = data?.metrics || []
  const totals = useMemo(() => metrics.reduce((acc, point) => ({
    tokens: acc.tokens + (point.total_tokens ?? 0),
    input: acc.input + getTotalInputTokens(point),
    cacheRead: acc.cacheRead + (point.cache_read_tokens ?? 0),
    cacheWrite: acc.cacheWrite + (point.cache_write_tokens ?? 0),
    output: acc.output + (point.completion_tokens ?? 0),
    calls: acc.calls + (point.total_requests ?? 0),
    promptCost: acc.promptCost + (point.prompt_cost ?? 0),
    cacheReadCost: acc.cacheReadCost + (point.cache_read_cost ?? 0),
    cacheWriteCost: acc.cacheWriteCost + (point.cache_write_cost ?? 0),
    outputCost: acc.outputCost + (point.completion_cost ?? 0),
  }), { tokens: 0, input: 0, cacheRead: 0, cacheWrite: 0, output: 0, calls: 0, promptCost: 0, cacheReadCost: 0, cacheWriteCost: 0, outputCost: 0 }), [metrics])
  const rawTokenCost = data?.raw_token_cost ?? 0
  const helixCredits = data?.helix_credits ?? 0
  const cacheSavings = data?.cache_savings ?? 0
  const subscriptionSavings = data?.subscription_savings ?? 0
  const compute = data?.compute
  const sessions = data?.active_sessions ?? 0
  const activeDays = metrics.filter(point => (point.total_tokens ?? 0) > 0).length
  const uncachedInput = Math.max(totals.input - totals.cacheRead - totals.cacheWrite, 0)

  const providers = useMemo(
    () => [...(data?.providers || [])].sort((a, b) => metricOf(b, metric) - metricOf(a, metric)),
    [data?.providers, metric],
  )
  const providerTotal = providers.reduce((sum, row) => sum + metricOf(row, metric), 0)
  const groups = useMemo(() => groupProviders(data?.providers || [], metric), [data?.providers, metric])
  const days = useMemo(() => enumerateDays(from, to), [from, to])
  const bars = useMemo(() => buildDailyBars(
    days,
    (data?.provider_time_series || []).map(series => ({ key: series.provider || 'unknown', metrics: series.metrics })),
    metric,
  ), [days, data?.provider_time_series, metric])

  const models = useMemo(
    () => [...(data?.models || [])]
      .filter(row => metricOf(row, 'tokens') > 0)
      .sort((a, b) => metricOf(b, metric) - metricOf(a, metric) || metricOf(b, 'tokens') - metricOf(a, 'tokens')),
    [data?.models, metric],
  )
  const modelTotal = models.reduce((sum, row) => sum + metricOf(row, metric), 0)
  const shownModels = showAllModels ? models : models.slice(0, MODEL_LIMIT)

  const unsplitCost = rawTokenCost - totals.promptCost - totals.cacheReadCost - totals.cacheWriteCost - totals.outputCost
  const typeSegments: UsageSegment[] = metric === 'cost'
    ? [
      { label: 'Uncached input', value: totals.promptCost, color: TYPE_COLORS.input },
      { label: 'Cache read', value: totals.cacheReadCost, color: TYPE_COLORS.cacheRead },
      { label: 'Cache write', value: totals.cacheWriteCost, color: TYPE_COLORS.cacheWrite },
      { label: 'Output', value: totals.outputCost, color: TYPE_COLORS.output },
      // Cost with no per-type split (estimated from list rates, or reported as
      // one figure), so the bar adds up to the headline. Below a cent it is rounding.
      { label: 'Other', value: unsplitCost >= 0.005 ? unsplitCost : 0, color: '#64748b' },
    ]
    : [
      { label: 'Uncached input', value: uncachedInput, color: TYPE_COLORS.input },
      { label: 'Cache read', value: totals.cacheRead, color: TYPE_COLORS.cacheRead },
      { label: 'Cache write', value: totals.cacheWrite, color: TYPE_COLORS.cacheWrite },
      { label: 'Output', value: totals.output, color: TYPE_COLORS.output },
    ]
  const sourceSegments: UsageSegment[] = groups.map(group => ({
    label: PROVIDER_KIND_GROUP_LABEL[group.kind],
    value: group.total,
    color: PROVIDER_KIND_COLOR[group.kind],
  }))
  const format = (value: number) => formatMetric(value, metric)

  const heroDetail = metric === 'cost'
    ? costMode === 'billed'
      ? `${formatCost(helixCredits)} debited from credits`
      : 'At API list rates; not charged'
    : `Across ${formatNumber(sessions)} ${sessions === 1 ? 'session' : 'sessions'}`

  const tabs = (
    <SegmentedTabs<UsageSummaryView>
      label="Usage view"
      value={view}
      onChange={onViewChange}
      options={[
        { value: 'cost', label: 'Cost', disabled: costMode === 'unpriced' },
        { value: 'tokens', label: 'Tokens' },
        { value: 'limits', label: 'Limits' },
      ]}
    />
  )

  if (view === 'limits') {
    return (
      <Stack spacing={3}>
        <Box sx={{ maxWidth: { md: 480 } }}>{tabs}</Box>
        <UsageLimits orgId={orgId} />
      </Stack>
    )
  }

  return (
    <Stack spacing={3}>
      <Box sx={{ maxWidth: { md: 480 } }}>{tabs}</Box>

      <Box
        sx={{
          display: 'grid',
          gap: 3,
          gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(0, 1fr) 380px' },
          alignItems: 'start',
        }}
      >
        <HeroCard
          label={metric === 'cost' ? 'API-rate cost' : 'Processed tokens'}
          value={metric === 'cost' ? formatCost(rawTokenCost) : formatCompact(totals.tokens)}
          detail={heroDetail}
          bars={bars}
          providers={groups.flatMap(group => group.rows)}
          colors={providerColors}
          metric={metric}
        />
        {groups.length > 0 && (
          <SummarySection title="Providers">
            <ProvidersCard groups={groups} colors={providerColors} metric={metric} total={providerTotal} />
          </SummarySection>
        )}
      </Box>

      <SummarySection title="Totals">
        <SummaryCard>
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(4, minmax(0, 1fr))' },
              '& > *': { borderColor: 'divider' },
            }}
          >
            <MetricCell
              label="Processed tokens"
              value={formatCompact(totals.tokens)}
              detail={activeDays > 0 ? `${formatCompact(totals.tokens / activeDays)} per active day` : 'No LLM calls'}
            />
            <MetricCell label="LLM calls" value={formatNumber(totals.calls)} detail={`${activeDays} ${activeDays === 1 ? 'day' : 'days'} with calls`} />
            <MetricCell
              label="Cached input"
              value={formatCompact(totals.cacheRead)}
              detail={`${formatPercent(totals.input > 0 ? totals.cacheRead / totals.input : 0)} of input`}
            />
            <MetricCell label="Output" value={formatCompact(totals.output)} detail={`${formatCompact(uncachedInput)} uncached input`} />
            <MetricCell
              label="Cache savings"
              value={formatCost(cacheSavings)}
              detail={rawTokenCost > 0 ? `${(cacheSavings / rawTokenCost).toFixed(1)}× the API-rate cost` : 'vs full input rates'}
            />
            <MetricCell
              label="Subscription savings"
              value={formatCost(subscriptionSavings)}
              detail={rawTokenCost > 0 ? `${formatPercent(subscriptionSavings / rawTokenCost)} of API-rate cost` : 'No subscription usage'}
            />
            <MetricCell
              label="Sandbox runtime"
              value={compute ? formatHours(compute.total_hours) : '—'}
              detail={compute
                ? (compute.total_credits ?? 0) > 0 ? `${formatCost(compute.total_credits)} compute` : `${compute.running_sandboxes ?? 0} running now`
                : 'No compute in this range'}
            />
            {costMode === 'billed' ? (
              <MetricCell label="Credits debited" value={formatCost(helixCredits)} detail={`≈ ${formatCost(rawTokenCost)} at API rates`} />
            ) : (
              <MetricCell label="Sessions" value={formatNumber(sessions)} detail={`${formatNumber(data?.active_projects)} projects`} />
            )}
          </Box>
        </SummaryCard>
      </SummarySection>

      {(providerTotal > 0 || typeSegments.some(segment => segment.value > 0)) && (
        <SummarySection title={metric === 'cost' ? 'Cost' : 'Tokens'}>
          <SummaryCard sx={{ p: 2, display: 'grid', gap: 3, gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: 'repeat(2, minmax(0, 1fr))' } }}>
            <SegmentBar label="By type" segments={typeSegments} format={format} />
            <SegmentBar label="By source" segments={sourceSegments} format={format} />
          </SummaryCard>
        </SummarySection>
      )}

      {models.length > 0 && (
        <SummarySection
          title="By model"
          action={(
            <Button
              size="small"
              color="inherit"
              endIcon={<ChevronRight size={16} />}
              onClick={models.length > MODEL_LIMIT && !showAllModels ? () => setShowAllModels(true) : onShowAllModels}
              sx={{ textTransform: 'none', color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
            >
              {models.length > MODEL_LIMIT && !showAllModels ? `Show all ${models.length}` : 'Model details'}
            </Button>
          )}
        >
          <ModelsCard models={shownModels} metric={metric} total={modelTotal} />
        </SummarySection>
      )}
    </Stack>
  )
}

export default UsageSummary
