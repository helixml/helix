import React, { FC, useEffect, useMemo, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import TablePagination from '@mui/material/TablePagination'
import { alpha } from '@mui/material/styles'
import InputAdornment from '@mui/material/InputAdornment'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import Autocomplete from '@mui/material/Autocomplete'
import LinearProgress from '@mui/material/LinearProgress'
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { TooltipContentProps, YAxisTickContentProps } from 'recharts'
import { Cloud, Download, Search, SlidersHorizontal, X } from 'lucide-react'

import type { TypesAggregatedUsageMetric, TypesUsageAgentRuntimeTimeSeries, TypesUsageBreakdownRow, TypesUsageFilterOption, TypesUsageModelTimeSeries, TypesUsageProviderTimeSeries } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useDebounce from '../../hooks/useDebounce'
import useLightTheme from '../../hooks/useLightTheme'
import Page from '../system/Page'
import SimpleTable, { ITableField } from '../widgets/SimpleTable'
import ShadcnAreaChart, { ShadcnSeries } from '../usage/ShadcnAreaChart'
import { PROVIDERS } from '../providers/types'
import { useGetOrgUsage } from '../../services/orgService'
import helixLogo from '../../../assets/img/logo.png'
import {
  buildCacheHitRatioChartData,
  getAggregateCacheHitRatio,
  getAggregateToolCallErrorRate,
  getCacheHitRatio,
  getCacheUsageSeriesKey,
  getToolCallErrorRate,
  getTotalInputTokens,
  getUncachedInputTokens,
} from '../../utils/usageMetrics'
import { initialUsageParam, toUsageDateInput, usageRangeFrom } from '../../utils/usageDateRange'
import { formatCompact, formatCost, formatNumber, formatPercent } from '../../utils/usageFormat'
import {
  OverviewStat,
  OverviewStatStrip,
  UsageChartGrid,
  UsageGroup,
  USAGE_RANGE_DAYS,
  UsagePageHeader,
  UsageRangeControls,
  UsageRangeKey,
  UsageSection,
  UsageShareRow,
} from '../usage/UsageOverview'

type UsageLoadingScope = 'filters' | 'projects' | 'tasks' | 'sessions' | 'users'
type OverviewMetric = 'cost' | 'tokens'
type LatencyMetric = 'per-request' | 'per-1k-output'
type UsageChartRow = { date: string; [key: string]: number | string }

const TOKEN_SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Input', color: '#2563eb' },
  { key: 'cacheRead', label: 'Cache read', color: '#16a34a' },
  { key: 'cacheWrite', label: 'Cache write', color: '#d97706' },
  { key: 'output', label: 'Output', color: '#9333ea' },
]

const CHART_COLORS = ['#2563eb', '#16a34a', '#d97706', '#9333ea', '#0891b2', '#e11d48']

// Two hues from the page palette so compute reads as part of the same system
// as the token charts above it. Desktops first: they are the expensive half.
const COMPUTE_SERIES: ShadcnSeries[] = [
  { key: 'desktop', label: 'Desktops', color: CHART_COLORS[0] },
  { key: 'headless', label: 'Headless', color: CHART_COLORS[4] },
]

const PROVIDER_COLORS: Record<string, string> = {
  anthropic: '#d97757',
  openai: '#e6e6e6',
  google: '#4285f4',
  gemini: '#4285f4',
  xai: '#ffffff',
  groq: '#f55036',
  cerebras: '#f59e0b',
  nvidia: '#76b900',
  togetherai: '#7c3aed',
  fireworks: '#ec4899',
  azure: '#0078d4',
  'amazon-bedrock': '#ff9900',
  deepseek: '#4d6bfe',
  helix: '#2563eb',
}

const toRFC3339 = (value: string, endOfDay = false) => {
  if (!value) return undefined
  return new Date(`${value}T${endOfDay ? '23:59:59.999' : '00:00:00.000'}Z`).toISOString()
}

const fromURLDate = (value: string | null, fallback: string) => {
  if (!value) return fallback
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return fallback
  return toUsageDateInput(parsed)
}

const currentSearchParams = () => new URLSearchParams(window.location.search)

const formatMs = (value?: number) => `${Math.round(value ?? 0).toLocaleString()} ms`

const formatDateTime = (value?: string) => value
  ? new Date(value).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
  : '-'

const filterOptionLabel = (option: TypesUsageFilterOption) => {
  const label = option.name || option.model || option.provider || option.id || ''
  if (option.email && option.email !== label) return `${label} (${option.email})`
  if (option.provider && option.model) return `${option.provider} / ${option.model}`
  return label
}

const providerKey = (provider?: string) => (provider || 'unknown').toLowerCase().replace(/^user\//, '')

const providerColor = (provider: string, index: number, isLight: boolean) => {
  const key = providerKey(provider)
  if (key === 'openai' || key === 'xai') return isLight ? '#111827' : PROVIDER_COLORS[key]
  if (key === 'helix' || key.startsWith('helix/')) return PROVIDER_COLORS.helix
  return PROVIDER_COLORS[key] || CHART_COLORS[index % CHART_COLORS.length]
}

const providerDefinition = (provider?: string) => {
  const rawKey = providerKey(provider)
  const key = rawKey === 'amazon-bedrock' ? 'aws' : rawKey
  return PROVIDERS.find(item => item.id === `user/${key}` || item.alias.includes(key))
}

export const UsageProviderIcon: FC<{ provider?: string; size?: number }> = ({ provider, size = 16 }) => {
  const key = providerKey(provider)
  if (key === 'helix' || key.startsWith('helix/')) {
    return <Box component="img" src={helixLogo} alt="" sx={{ width: size, height: size, objectFit: 'contain' }} />
  }
  const definition = providerDefinition(provider)
  if (!definition) return <Cloud size={size} aria-hidden="true" />
  if (typeof definition.logo === 'string') {
    return <Box component="img" src={definition.logo} alt="" sx={{ width: size, height: size, objectFit: 'contain' }} />
  }
  const Logo = definition.logo
  return <Logo width={size} height={size} aria-hidden="true" />
}

const FilterAutocomplete: FC<{
  label: string
  options: TypesUsageFilterOption[]
  value: TypesUsageFilterOption | null
  onChange: (option: TypesUsageFilterOption | null) => void
  loading?: boolean
}> = ({ label, options, value, onChange, loading }) => (
  <Autocomplete
    size="small"
    options={options}
    value={value}
    loading={loading}
    onChange={(_, option) => onChange(option)}
    getOptionLabel={filterOptionLabel}
    isOptionEqualToValue={(option, selected) => option.id === selected.id}
    renderInput={(params) => <TextField {...params} label={label} />}
  />
)

const sumMetrics = (metrics: TypesAggregatedUsageMetric[] = []) => metrics.reduce((acc, metric) => ({
  input: acc.input + getTotalInputTokens(metric),
  completion: acc.completion + (metric.completion_tokens ?? 0),
  cacheRead: acc.cacheRead + (metric.cache_read_tokens ?? 0),
  cacheWrite: acc.cacheWrite + (metric.cache_write_tokens ?? 0),
  total: acc.total + (metric.total_tokens ?? 0),
  cost: acc.cost + (metric.total_cost ?? 0),
  requests: acc.requests + (metric.total_requests ?? 0),
}), { input: 0, completion: 0, cacheRead: 0, cacheWrite: 0, total: 0, cost: 0, requests: 0 })

const csvEscape = (value: string | number | undefined) => `"${String(value ?? '').replace(/"/g, '""')}"`

const exportRows = (filename: string, rows: TypesUsageBreakdownRow[]) => {
  const headers = [
    'id',
    'name',
    'email',
    'username',
    'provider',
    'model',
    'session_id',
    'requests',
    'sessions',
    'unique_users',
    'unique_projects',
    'unique_apps',
    'input_tokens',
    'output_tokens',
    'cache_read_tokens',
    'cache_write_tokens',
    'cache_hit_ratio_percent',
    'total_tokens',
    'input_cost',
    'output_cost',
    'cache_read_cost',
    'cache_write_cost',
    'total_cost',
    'latency_ms',
    'last_activity_at',
  ]
  const lines = [
    headers.join(','),
    ...rows.map(row => {
      const cacheHitRatio = getCacheHitRatio(row)
      return [
        row.id,
        row.name,
        row.email,
        row.username,
        row.provider,
        row.model,
        row.session_id,
        row.total_requests,
        row.session_count,
        row.unique_users,
        row.unique_projects,
        row.unique_apps,
        getTotalInputTokens(row),
        row.completion_tokens,
        row.cache_read_tokens,
        row.cache_write_tokens,
        cacheHitRatio === null ? undefined : cacheHitRatio * 100,
        row.total_tokens,
        row.prompt_cost,
        row.completion_cost,
        row.cache_read_cost,
        row.cache_write_cost,
        row.total_cost,
        row.latency_ms,
        row.last_activity_at,
      ].map(csvEscape).join(',')
    }),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

const exportJSON = (filename: string, rows: TypesUsageBreakdownRow[]) => {
  const exportedRows = rows.map(row => ({
    ...row,
    input_tokens: getTotalInputTokens(row),
    cache_hit_ratio: getCacheHitRatio(row),
  }))
  const blob = new Blob([JSON.stringify(exportedRows, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

const ProviderSummaryRow: FC<{
  provider: TypesUsageBreakdownRow
  metric: OverviewMetric
  total: number
  color: string
}> = ({ provider, metric, total, color }) => {
  const value = metric === 'cost' ? (provider.total_cost ?? 0) : (provider.total_tokens ?? 0)
  const share = total > 0 ? value / total : 0
  return (
    <UsageShareRow
      icon={<UsageProviderIcon provider={provider.provider} />}
      label={provider.name || provider.provider || 'Unknown'}
      value={metric === 'cost' ? formatCost(value) : formatCompact(value)}
      share={share}
      color={color}
      caption={`${formatPercent(share)} of ${metric} · ${formatCompact(provider.total_tokens)} tokens`}
    />
  )
}

// UsagePanel is the outlined card the Sandbox compute block uses. Everything
// on the page sits in one, so the charts and tables read as parts of a single
// system instead of floating panes on the page background.
const UsagePanel: FC<{ children: React.ReactNode }> = ({ children }) => (
  <Paper variant="outlined" sx={{ p: 2, overflow: 'hidden' }}>
    {children}
  </Paper>
)

// BreakdownTable collapses an empty breakdown to one muted line instead of a
// table header, "No data" row and pagination footer.
const BreakdownTable: FC<{ empty: string; rows: unknown[]; loading?: boolean; children: React.ReactNode }> = ({ empty, rows, loading, children }) => (
  rows.length === 0 && !loading
    ? <Typography variant="body2" color="text.secondary">{empty}</Typography>
    : <>{children}</>
)

// TableScroll lets wide breakdown tables scroll sideways on narrow screens
// instead of being clipped by their card.
// Edge shadows (attached to the content vs the box) appear only while more
// columns are scrolled out of view.
const TableScroll: FC<{ children: React.ReactNode }> = ({ children }) => (
  <Box
    sx={theme => {
      const paper = theme.palette.background.paper
      const shade = alpha(theme.palette.text.primary, 0.12)
      return {
        overflowX: 'auto',
        mx: -2,
        px: 2,
        background: `linear-gradient(to right, ${paper} 30%, ${alpha(paper, 0)}) left center / 32px 100% no-repeat local,
          linear-gradient(to left, ${paper} 30%, ${alpha(paper, 0)}) right center / 32px 100% no-repeat local,
          radial-gradient(farthest-side at 0 50%, ${shade}, transparent) left center / 10px 100% no-repeat scroll,
          radial-gradient(farthest-side at 100% 50%, ${shade}, transparent) right center / 10px 100% no-repeat scroll`,
        '& .MuiTableContainer-root, & .MuiPaper-root, & table': { background: 'transparent', boxShadow: 'none' },
        '& th, & td': { whiteSpace: 'nowrap', px: 1.25 },
        '& td:first-of-type': { maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis' },
        '& td:first-of-type p, & td:first-of-type span': { overflow: 'hidden', textOverflow: 'ellipsis' },
      }
    }}
  >
    {children}
  </Box>
)

const ExportButtons: FC<{ filename: string; rows: TypesUsageBreakdownRow[] }> = ({ filename, rows }) => (
  <Stack direction="row" spacing={1}>
    <Button
      size="small"
      variant="outlined"
      startIcon={<Download size={16} />}
      onClick={() => exportRows(`${filename}.csv`, rows)}
      disabled={!rows.length}
    >
      CSV
    </Button>
    <Button
      size="small"
      variant="outlined"
      onClick={() => exportJSON(`${filename}.json`, rows)}
      disabled={!rows.length}
    >
      JSON
    </Button>
  </Stack>
)

// Cost and total tokens sit next to the name: they are what a breakdown is
// read for, and on a phone they stay in view before the table scrolls.
const baseFields: ITableField[] = [
  { name: 'name', title: 'Name' },
  { name: 'cost', title: 'Cost', numeric: true },
  { name: 'total', title: 'Tokens', numeric: true },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'sessions', title: 'Sessions', numeric: true },
  { name: 'input', title: 'Input', numeric: true },
  { name: 'output', title: 'Output', numeric: true },
  { name: 'cache', title: 'Cache read', numeric: true },
  { name: 'cacheHit', title: 'Hit ratio', numeric: true },
  { name: 'latency', title: 'Latency', numeric: true },
]

const modelOverviewFields: ITableField[] = [
  { name: 'name', title: 'Model' },
  { name: 'cost', title: 'API-rate cost', numeric: true },
  { name: 'share', title: 'Share', numeric: true },
  { name: 'tokens', title: 'Tokens', numeric: true },
]

const computeSandboxFields: ITableField[] = [
  { name: 'name', title: 'Sandbox' },
  // Spec-task runners inherit their task's name, so two projects in the same
  // org routinely produce identically-named rows. Project is what makes the
  // cost attributable.
  { name: 'project', title: 'Project' },
  { name: 'kind', title: 'Kind' },
  { name: 'size', title: 'Size', numeric: true },
  { name: 'credits', title: 'Credits', numeric: true },
  { name: 'share', title: 'Share', numeric: true },
]

const sessionFields: ITableField[] = [
  { name: 'name', title: 'Session' },
  { name: 'cost', title: 'Cost', numeric: true },
  { name: 'total', title: 'Tokens', numeric: true },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'users', title: 'Users', numeric: true },
  { name: 'projects', title: 'Projects', numeric: true },
  { name: 'input', title: 'Input', numeric: true },
  { name: 'output', title: 'Output', numeric: true },
  { name: 'cache', title: 'Cache read', numeric: true },
  { name: 'cacheHit', title: 'Hit ratio', numeric: true },
  { name: 'lastActivity', title: 'Last activity' },
]

const rowToTable = (row: TypesUsageBreakdownRow, withProvider = false, onNameClick?: (row: TypesUsageBreakdownRow) => void) => ({
  id: row.id,
  _data: row,
  name: (
    <Box>
      {onNameClick ? (
        <a
          href="#"
          onClick={e => {
            e.preventDefault()
            e.stopPropagation()
            onNameClick(row)
          }}
          style={{ color: 'inherit', textDecoration: 'none' }}
        >
          <Typography variant="body2" title={row.name || row.id} sx={{ fontWeight: 600, '&:hover': { textDecoration: 'underline' } }}>{row.name || row.id || 'Unknown'}</Typography>
        </a>
      ) : (
        <Typography variant="body2" title={row.name || row.id} sx={{ fontWeight: 600 }}>{row.name || row.id || 'Unknown'}</Typography>
      )}
      {(row.email || row.username) && (
        <Typography variant="caption" color="text.secondary">
          {[...new Set([row.email, row.username].filter(Boolean))].join(' · ')}
        </Typography>
      )}
      {row.model && !withProvider && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
          {row.provider ? `${row.provider} / ` : ''}{row.model}
        </Typography>
      )}
    </Box>
  ),
  provider: <Typography variant="body2" color="text.secondary">{row.provider || '-'}</Typography>,
  users: <Typography variant="body2">{formatNumber(row.unique_users)}</Typography>,
  projects: <Typography variant="body2">{formatNumber(row.unique_projects)}</Typography>,
  sessions: <Typography variant="body2">{formatNumber(row.session_count)}</Typography>,
  requests: <Typography variant="body2">{formatNumber(row.total_requests)}</Typography>,
  // Token counts are compact (20.4M) with the exact count on hover; exports keep exact values.
  input: <Typography variant="body2" title={formatNumber(getTotalInputTokens(row))}>{formatCompact(getTotalInputTokens(row))}</Typography>,
  output: <Typography variant="body2" title={formatNumber(row.completion_tokens)}>{formatCompact(row.completion_tokens)}</Typography>,
  cache: <Typography variant="body2" title={formatNumber(row.cache_read_tokens)}>{formatCompact(row.cache_read_tokens)}</Typography>,
  cacheHit: <Typography variant="body2">{formatPercent(getCacheHitRatio(row))}</Typography>,
  total: <Typography variant="body2" title={formatNumber(row.total_tokens)}>{formatCompact(row.total_tokens)}</Typography>,
  cost: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatCost(row.total_cost)}</Typography>,
  latency: <Typography variant="body2">{formatMs(row.latency_ms)}</Typography>,
  lastActivity: <Typography variant="body2" color="text.secondary">{formatDateTime(row.last_activity_at)}</Typography>,
})

const buildModelChart = (series: TypesUsageModelTimeSeries[] = []): UsageChartRow[] => {
  const dates = new Map<string, UsageChartRow>()
  series.slice(0, CHART_COLORS.length).forEach(model => {
    model.metrics?.forEach(metric => {
      if (!metric.date) return
      const entry = dates.get(metric.date) ?? { date: metric.date }
      entry[model.id || model.model || 'model'] = metric.total_tokens ?? 0
      dates.set(metric.date, entry)
    })
  })
  return Array.from(dates.values()).sort((a, b) => String(a.date).localeCompare(String(b.date)))
}

const buildProviderChart = (
  series: TypesUsageProviderTimeSeries[] = [],
  metric: OverviewMetric,
): UsageChartRow[] => {
  const dates = new Map<string, UsageChartRow>()
  series.forEach(provider => {
    provider.metrics?.forEach(value => {
      if (!value.date) return
      const row = dates.get(value.date) ?? { date: value.date }
      row[provider.provider || 'unknown'] = metric === 'cost' ? (value.total_cost ?? 0) : (value.total_tokens ?? 0)
      dates.set(value.date, row)
    })
  })
  return Array.from(dates.values()).sort((a, b) => String(a.date).localeCompare(String(b.date)))
}

const buildAgentCacheChart = (series: TypesUsageAgentRuntimeTimeSeries[] = []) => (
  buildCacheHitRatioChartData(series)
)

const buildProjectModelChart = (rows: TypesUsageBreakdownRow[] = [], projects: TypesUsageBreakdownRow[] = []) => {
  const topProjectNames = new Set(projects.slice(0, 10).map(project => project.name || project.id || 'Unassigned'))
  const filteredRows = rows.filter(row => topProjectNames.has(row.name || row.id || 'Unassigned'))
  const modelTotals = new Map<string, { key: string; label: string; total: number }>()

  filteredRows.forEach(row => {
    const key = `${row.provider || 'unknown'}:${row.model || 'unknown'}`
    const existing = modelTotals.get(key) || {
      key,
      label: row.model || row.provider || 'Unknown',
      total: 0,
    }
    existing.total += row.total_tokens || 0
    modelTotals.set(key, existing)
  })

  const series = Array.from(modelTotals.values())
    .sort((a, b) => b.total - a.total)
    .slice(0, CHART_COLORS.length)
    .map((model, index) => ({
      ...model,
      color: CHART_COLORS[index],
    }))

  const allowedModels = new Set(series.map(model => model.key))
  const byProject = new Map<string, Record<string, number | string>>()
  filteredRows.forEach(row => {
    const modelKey = `${row.provider || 'unknown'}:${row.model || 'unknown'}`
    if (!allowedModels.has(modelKey)) return
    const projectName = row.name || row.id || 'Unassigned'
    const entry = byProject.get(projectName) || { project: projectName }
    entry[modelKey] = Number(entry[modelKey] || 0) + (row.total_tokens || 0)
    byProject.set(projectName, entry)
  })

  const projectOrder = new Map(projects.map((project, index) => [project.name || project.id || 'Unassigned', index]))
  const data = Array.from(byProject.values()).sort((a, b) => {
    return (projectOrder.get(String(a.project)) ?? 999) - (projectOrder.get(String(b.project)) ?? 999)
  })

  return { data, series }
}

const ProjectModelTooltip: FC<TooltipContentProps<number, string>> = ({ active, payload, label }) => {
  const lightTheme = useLightTheme()
  if (!active || !payload?.length) return null
  return (
    <Box
      sx={{
        bgcolor: lightTheme.isLight ? 'rgba(255,255,255,0.98)' : 'rgba(10,10,15,0.95)',
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 2,
        boxShadow: lightTheme.isLight ? '0 4px 12px rgba(0,0,0,0.12)' : '0 4px 12px rgba(0,0,0,0.4)',
        px: 1.5,
        py: 1,
        minWidth: 220,
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
        {label}
      </Typography>
      {payload.map(item => {
        const value = Number(item.value || 0)
        if (!value) return null
        return (
          <Box key={item.dataKey as string} sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.25 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: '2px', bgcolor: item.color }} />
            <Typography variant="caption" sx={{ flex: 1 }}>{item.name}</Typography>
            <Typography variant="caption" sx={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>
              {formatCompact(value)}
            </Typography>
          </Box>
        )
      })}
    </Box>
  )
}

const PROJECT_ROW_HEIGHT = 40

// ProjectModelChart ranks the top projects as horizontal bars, each split by
// the models that processed its tokens, so long project names stay readable.
const ProjectModelChart: FC<{
  data: Array<Record<string, number | string>>,
  series: Array<{ key: string; label: string; color: string }>,
}> = ({ data, series }) => {
  const lightTheme = useLightTheme()
  const hasData = data.some(row => series.some(item => Number(row[item.key] || 0) > 0))
  const tick = { fill: lightTheme.isLight ? '#475569' : '#94A3B8', fontSize: 12 }
  const longest = Math.max(0, ...data.map(row => String(row.project).length))
  return (
    <Box sx={{ bgcolor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
      <Box sx={{ mb: 1 }}>
        <Typography variant="body2" sx={{ fontWeight: 600 }}>Project tokens by model</Typography>
        <Typography variant="caption" color="text.secondary">Top projects, split by the models that processed their tokens</Typography>
      </Box>
      {hasData ? (
        <ResponsiveContainer width="100%" height={data.length * PROJECT_ROW_HEIGHT + 64}>
          <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, left: 0, bottom: 0 }} barCategoryGap={10}>
            <CartesianGrid horizontal={false} stroke={lightTheme.isLight ? 'rgba(15,23,42,0.08)' : 'rgba(255,255,255,0.08)'} />
            <XAxis type="number" tickLine={false} axisLine={false} tick={tick} tickFormatter={formatCompact} />
            <YAxis
              type="category"
              dataKey="project"
              tickLine={false}
              axisLine={false}
              width={Math.min(240, Math.max(96, longest * 7 + 12))}
              // A plain <text> tick: recharts' default wraps long names onto two lines.
              tick={(props: YAxisTickContentProps) => {
                const name = String(props.payload?.value ?? '')
                return (
                  <text x={Number(props.x) - 6} y={Number(props.y)} dy={4} textAnchor="end" fill={tick.fill} fontSize={tick.fontSize}>
                    <title>{name}</title>
                    {name.length > 32 ? `${name.slice(0, 31)}…` : name}
                  </text>
                )
              }}
            />
            <Tooltip
              content={React.createElement(ProjectModelTooltip)}
              cursor={{ fill: lightTheme.isLight ? 'rgba(15,23,42,0.04)' : 'rgba(255,255,255,0.04)' }}
            />
            <Legend wrapperStyle={{ fontSize: '0.75rem' }} iconType="square" iconSize={8} />
            {series.map(item => (
              <Bar key={item.key} dataKey={item.key} name={item.label} stackId="tokens" fill={item.color} isAnimationActive={false} />
            ))}
          </BarChart>
        </ResponsiveContainer>
      ) : (
        <Typography variant="body2" color="text.secondary" sx={{ py: 3 }}>No project or model usage in this range.</Typography>
      )}
    </Box>
  )
}

const OrgUsage: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const orgID = router.params.org_id as string
  const today = toUsageDateInput(new Date())
  const initialParams = useMemo(() => currentSearchParams(), [])
  const initialParam = (key: string) => initialUsageParam(router.params, initialParams, key)
  const [range, setRange] = useState<UsageRangeKey | null>(() => (initialParam('from') || initialParam('to') ? null : '7d'))
  const [from, setFrom] = useState(() => fromURLDate(initialParam('from'), usageRangeFrom(7)))
  const [to, setTo] = useState(() => fromURLDate(initialParam('to'), today))
  const [userId, setUserId] = useState(() => initialParam('user_id'))
  const [projectId, setProjectId] = useState(() => initialParam('project_id'))
  const [taskId, setTaskId] = useState(() => initialParam('task_id'))
  const [appId, setAppId] = useState(() => initialParam('app_id'))
  const [sessionIdInput, setSessionIdInput] = useState(() => initialParam('session_id'))
  const [provider, setProvider] = useState(() => initialParam('provider'))
  const [model, setModel] = useState(() => initialParam('model'))
  // On a phone the filters fold behind one button so the numbers come first.
  const [filtersOpen, setFiltersOpen] = useState(false)
  const activeFilters = [userId, projectId, taskId, appId, sessionIdInput, provider || model].filter(Boolean).length
  const [userSearchInput, setUserSearchInput] = useState(() => initialParam('user_search'))
  const [userPage, setUserPage] = useState(0)
  const [userRowsPerPage, setUserRowsPerPage] = useState(10)
  const [projectPage, setProjectPage] = useState(0)
  const [projectRowsPerPage, setProjectRowsPerPage] = useState(10)
  const [taskPage, setTaskPage] = useState(0)
  const [taskRowsPerPage, setTaskRowsPerPage] = useState(10)
  const [sessionPage, setSessionPage] = useState(0)
  const [sessionRowsPerPage, setSessionRowsPerPage] = useState(10)
  const [overviewMetric, setOverviewMetric] = useState<OverviewMetric>('cost')
  const [latencyMetric, setLatencyMetric] = useState<LatencyMetric>('per-request')
  const [loadingScope, setLoadingScope] = useState<UsageLoadingScope | null>(null)
  const sessionId = useDebounce(sessionIdInput.trim(), 400)
  const userSearch = useDebounce(userSearchInput.trim(), 300)

  const fromRFC = useMemo(() => toRFC3339(from), [from])
  const toRFC = useMemo(() => toRFC3339(to, true), [to])

  const usage = useGetOrgUsage(orgID, {
    from: fromRFC,
    to: toRFC,
    userId: userId || undefined,
    projectId: projectId || undefined,
    taskId: taskId || undefined,
    appId: appId || undefined,
    sessionId: sessionId || undefined,
    provider: provider || undefined,
    model: model || undefined,
    userSearch,
    userLimit: userRowsPerPage,
    userOffset: userPage * userRowsPerPage,
    projectLimit: projectRowsPerPage,
    projectOffset: projectPage * projectRowsPerPage,
    taskLimit: taskRowsPerPage,
    taskOffset: taskPage * taskRowsPerPage,
    sessionLimit: sessionRowsPerPage,
    sessionOffset: sessionPage * sessionRowsPerPage,
    enabled: Boolean(orgID),
  })

  useEffect(() => {
    const url = new URL(window.location.href)
    const setParam = (key: string, value: string, defaultValue = '') => {
      if (value && value !== defaultValue) {
        url.searchParams.set(key, value)
      } else {
        url.searchParams.delete(key)
      }
    }

    setParam('from', from)
    setParam('to', to)
    setParam('user_id', userId)
    setParam('project_id', projectId)
    setParam('task_id', taskId)
    setParam('app_id', appId)
    setParam('session_id', sessionIdInput.trim())
    setParam('provider', provider)
    setParam('model', model)
    setParam('user_search', userSearchInput.trim())
    url.searchParams.delete('user_page')
    url.searchParams.delete('user_rows')
    url.searchParams.delete('project_page')
    url.searchParams.delete('project_rows')
    url.searchParams.delete('task_page')
    url.searchParams.delete('task_rows')
    url.searchParams.delete('session_page')
    url.searchParams.delete('session_rows')

    window.history.replaceState({}, '', url.toString())
  }, [from, to, userId, projectId, taskId, appId, sessionIdInput, provider, model, userSearchInput])

  useEffect(() => {
    if (!usage.isFetching) {
      setLoadingScope(null)
    }
  }, [usage.isFetching])

  const metrics = usage.data?.metrics || []
  const totals = useMemo(() => sumMetrics(metrics), [metrics])
  const rawTokenCost = usage.data?.raw_token_cost ?? totals.cost
  const subscriptionSavings = usage.data?.subscription_savings ?? 0
  const cacheSavings = usage.data?.cache_savings ?? 0
  const helixCredits = usage.data?.helix_credits ?? 0
  const compute = usage.data?.compute
  const computeTotals = useMemo(() => {
    const daily = compute?.daily || []
    const computeActiveDays = daily.filter(point => (point.total ?? 0) > 0).length
    const total = compute?.total_credits ?? 0
    return {
      total,
      desktop: compute?.desktop_credits ?? 0,
      headless: compute?.headless_credits ?? 0,
      activeDays: computeActiveDays,
      perActiveDay: computeActiveDays > 0 ? total / computeActiveDays : 0,
    }
  }, [compute])
  const computeChartData = useMemo<UsageChartRow[]>(() => (compute?.daily || []).map(point => ({
    date: point.date || '',
    desktop: point.desktop ?? 0,
    headless: point.headless ?? 0,
  })), [compute])
  // Project names come from the filter options the same response already
  // carries, so no extra request is needed to attribute a sandbox.
  const projectNames = useMemo(() => {
    const names: Record<string, string> = {}
    for (const option of usage.data?.filter_projects || []) {
      if (option.id) names[option.id] = option.name || option.id
    }
    return names
  }, [usage.data?.filter_projects])
  const computeSandboxRows = useMemo(() => (compute?.sandboxes || []).map(row => {
    const share = computeTotals.total > 0 ? (row.credits ?? 0) / computeTotals.total : 0
    return {
      id: row.sandbox_id || '',
      _data: row,
      // Project separates same-named tasks across projects, but two tasks in
      // ONE project can also share a name. The task link is what makes every
      // row individually resolvable.
      name: row.spec_task_id && row.project_id ? (
        <Typography
          variant="body2"
          component="a"
          href="#"
          onClick={(e: React.MouseEvent) => {
            e.preventDefault()
            e.stopPropagation()
            router.navigate('org_project-task-detail', {
              org_id: router.params.org_id,
              id: row.project_id,
              taskId: row.spec_task_id,
            })
          }}
          sx={{ fontWeight: 600, color: 'text.primary', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
          noWrap
        >
          {row.name || row.sandbox_id || 'Unknown'}
        </Typography>
      ) : (
        <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>
          {row.name || row.sandbox_id || 'Unknown'}
        </Typography>
      ),
      project: (
        <Typography variant="body2" color="text.secondary" noWrap>
          {row.project_id ? (projectNames[row.project_id] || row.project_id) : 'Org-scoped'}
        </Typography>
      ),
      kind: (
        <Typography variant="body2" color="text.secondary">
          {row.spec_task_id ? 'Spec task' : row.pricing_type === 'desktop' ? 'Desktop' : 'Headless'}
        </Typography>
      ),
      size: (
        <Typography variant="body2" color="text.secondary">
          {row.vcpus ? `${row.vcpus} vCPU` : '—'}
        </Typography>
      ),
      credits: <Typography variant="body2">{formatCost(row.credits)}</Typography>,
      share: <Typography variant="body2" color="text.secondary">{formatPercent(share)}</Typography>,
    }
  }), [compute, computeTotals.total, projectNames, router])

  const activeDays = metrics.filter(metric => (metric.total_tokens ?? 0) > 0).length
  const averageTokens = activeDays > 0 ? totals.total / activeDays : 0
  const cachedShare = totals.input > 0 ? totals.cacheRead / totals.input : 0
  const tokenChartData = useMemo(() => metrics.map(metric => {
    const cacheRead = metric.cache_read_tokens ?? 0
    const cacheWrite = metric.cache_write_tokens ?? 0
    return {
      date: metric.date || '',
      input: getUncachedInputTokens(metric),
      output: metric.completion_tokens ?? 0,
      cacheRead,
      cacheWrite,
    }
  }), [metrics])
  const cacheHitRatio = useMemo(() => getAggregateCacheHitRatio(metrics), [metrics])

  const providerTimeSeries = usage.data?.provider_time_series || []
  const providerChartData = buildProviderChart(providerTimeSeries, overviewMetric)
  const providerChartSeries: ShadcnSeries[] = providerTimeSeries.map((item, index) => ({
    key: item.provider || 'unknown',
    label: item.name || item.provider || 'Unknown',
    color: providerColor(item.provider || 'unknown', index, lightTheme.isLight),
  }))
  const providerRows = [...(usage.data?.providers || [])].sort((a, b) => overviewMetric === 'cost'
    ? (b.total_cost ?? 0) - (a.total_cost ?? 0)
    : (b.total_tokens ?? 0) - (a.total_tokens ?? 0))
  const providerTotal = providerRows.reduce((sum, row) => sum + (
    overviewMetric === 'cost' ? (row.total_cost ?? 0) : (row.total_tokens ?? 0)
  ), 0)

  const agentSeries = (usage.data?.agent_runtime_time_series || []).slice(0, CHART_COLORS.length)
  const agentCacheChartData = buildAgentCacheChart(agentSeries)
  const agentCacheChartSeries: ShadcnSeries[] = agentSeries.map((agent, index) => ({
    key: getCacheUsageSeriesKey(agent, index),
    label: agent.name || agent.runtime || 'Unattributed',
    color: CHART_COLORS[index],
  }))

  const modelSeries = (usage.data?.model_time_series || []).slice(0, CHART_COLORS.length)
  const modelChartData = useMemo(() => buildModelChart(modelSeries), [modelSeries])
  const modelChartSeries: ShadcnSeries[] = modelSeries.map((model, index) => ({
    key: model.id || model.model || `model-${index}`,
    label: model.name || model.model || `Model ${index + 1}`,
    color: CHART_COLORS[index],
  }))

  // Latency per provider. Mean request duration answers "how long does a call
  // take", but it tracks response length as much as provider speed — a
  // provider used for long agentic turns looks slow even when it streams
  // faster. Normalising by output tokens is the comparison that actually
  // ranks providers, so both are offered.
  const latencyChartData = useMemo<UsageChartRow[]>(() => {
    const byDate = new Map<string, UsageChartRow>()
    for (const series of providerTimeSeries) {
      const key = series.provider || 'unknown'
      for (const metric of series.metrics || []) {
        const date = metric.date || ''
        let row = byDate.get(date)
        if (!row) {
          row = { date }
          byDate.set(date, row)
        }
        // The API emits a zero-filled point for every provider on every date.
        // Plotting those zeros drags each line to the floor and back on days a
        // provider simply wasn't used, which reads as wild latency swings.
        // Leave the key unset instead so the point is absent, and let the
        // chart bridge the gap.
        const requests = metric.total_requests ?? 0
        if (requests === 0) continue
        const meanMs = metric.latency_ms ?? 0
        if (latencyMetric === 'per-request') {
          row[key] = meanMs
          continue
        }
        // Reconstruct total time from the mean the API publishes, then divide
        // by output volume.
        const completion = metric.completion_tokens ?? 0
        if (completion <= 0) continue
        row[key] = (meanMs * requests) / (completion / 1000)
      }
    }
    return Array.from(byDate.values()).sort((a, b) => String(a.date).localeCompare(String(b.date)))
  }, [providerTimeSeries, latencyMetric])
  const latencyChartSeries: ShadcnSeries[] = providerTimeSeries.map((item, index) => ({
    key: item.provider || 'unknown',
    label: item.name || item.provider || 'Unknown',
    color: providerColor(item.provider || 'unknown', index, lightTheme.isLight),
  }))
  // Tool call error rate per provider: of the requests that came back with
  // tool calls, how many contained a call the caller could not dispatch —
  // unparseable arguments, a tool that was never offered, or arguments that
  // miss the tool's schema. It is a provider-quality signal, not an agent one:
  // the same model behind two endpoints can differ here.
  const toolCallErrorChartData = useMemo<UsageChartRow[]>(() => {
    const byDate = new Map<string, UsageChartRow>()
    for (const series of providerTimeSeries) {
      const key = series.provider || 'unknown'
      for (const metric of series.metrics || []) {
        const date = metric.date || ''
        let row = byDate.get(date)
        if (!row) {
          row = { date }
          byDate.set(date, row)
        }
        // Days below the volume floor return null and stay absent from the
        // row. That covers the zero-filled points the API emits for providers
        // that weren't used that day, which would otherwise plot as a
        // confident 0% rather than "no data".
        const rate = getToolCallErrorRate(metric)
        if (rate === null) continue
        row[key] = rate
      }
    }
    return Array.from(byDate.values()).sort((a, b) => String(a.date).localeCompare(String(b.date)))
  }, [providerTimeSeries])
  const toolCallErrorHeadline = useMemo(() => formatPercent(
    getAggregateToolCallErrorRate(providerTimeSeries.flatMap(series => series.metrics || [])),
  ), [providerTimeSeries])

  const latencyHeadline = useMemo(() => {
    const values = latencyChartData.flatMap(row => latencyChartSeries
      .map(s => Number(row[s.key]) || 0)
      .filter(v => v > 0))
    if (!values.length) return '—'
    return formatMs(values.reduce((a, b) => a + b, 0) / values.length)
  }, [latencyChartData, latencyChartSeries])
  const userOptions = usage.data?.filter_users || []
  const projectOptions = usage.data?.filter_projects || []
  const taskOptions = usage.data?.filter_tasks || []
  const appOptions = usage.data?.filter_apps || []
  const modelOptions = usage.data?.filter_models || []
  const providerOptions = useMemo(() => {
    const providers = new Map<string, TypesUsageFilterOption>()
    modelOptions.forEach(option => {
      if (!option.provider) return
      providers.set(option.provider, {
        id: option.provider,
        name: option.provider,
        provider: option.provider,
      })
    })
    return Array.from(providers.values()).sort((a, b) => (a.name || '').localeCompare(b.name || ''))
  }, [modelOptions])
  const filteredModelOptions = useMemo(() => {
    if (!provider) return modelOptions
    return modelOptions.filter(option => option.provider === provider)
  }, [modelOptions, provider])
  const selectedUser = userOptions.find(option => option.id === userId) || null
  const selectedProject = projectOptions.find(option => option.id === projectId) || null
  const selectedTask = taskOptions.find(option => option.id === taskId) || null
  const selectedApp = appOptions.find(option => option.id === appId) || null
  const selectedProvider = providerOptions.find(option => option.id === provider) || null
  const selectedModel = filteredModelOptions.find(option => option.provider === provider && option.model === model) || null
  const projectModelChart = useMemo(() => {
    return buildProjectModelChart(usage.data?.project_models || [], usage.data?.projects || [])
  }, [usage.data?.project_models, usage.data?.projects])

  const resetPagedTables = () => {
    setUserPage(0)
    setProjectPage(0)
    setTaskPage(0)
    setSessionPage(0)
  }

  const markFilterChange = () => {
    setLoadingScope('filters')
    resetPagedTables()
  }

  const isScopedLoading = (scope: UsageLoadingScope) => loadingScope === scope && usage.isFetching && !usage.isLoading

  const handleRangeChange = (next: UsageRangeKey) => {
    markFilterChange()
    setRange(next)
    setFrom(usageRangeFrom(USAGE_RANGE_DAYS[next]))
    setTo(today)
  }

  const clearFilters = () => {
    markFilterChange()
    setUserId('')
    setProjectId('')
    setTaskId('')
    setAppId('')
    setSessionIdInput('')
    setProvider('')
    setModel('')
    setUserSearchInput('')
  }

  const openProject = (row: TypesUsageBreakdownRow) => {
    const id = row.id
    if (!id) return
    account.orgNavigate('project-specs', { id })
  }

  const openAgent = (row: TypesUsageBreakdownRow) => {
    const id = row.id
    if (!id) return
    account.orgNavigate('agent', { app_id: id })
  }

  const tableRows = {
    projects: (usage.data?.projects || []).map(row => rowToTable(row, false, openProject)),
    apps: (usage.data?.apps || []).map(row => rowToTable(row, false, openAgent)),
    tasks: (usage.data?.tasks || []).map(row => rowToTable(row)),
    sessions: (usage.data?.sessions || []).map(row => rowToTable(row)),
    users: (usage.data?.users || []).map(row => rowToTable(row)),
  }
  const modelOverviewRows = (usage.data?.models || []).map(row => {
    const costShare = rawTokenCost > 0 ? (row.total_cost ?? 0) / rawTokenCost : 0
    return {
      id: row.id,
      _data: row,
      name: (
        <Stack direction="row" alignItems="center" spacing={1}>
          <UsageProviderIcon provider={row.provider} size={15} />
          <Typography variant="body2" sx={{ fontWeight: 600 }}>{row.model || row.name || 'Unknown'}</Typography>
        </Stack>
      ),
      cost: <Typography variant="body2">{formatCost(row.total_cost)}</Typography>,
      share: <Typography variant="body2" color="text.secondary">{formatPercent(costShare)}</Typography>,
      tokens: <Typography variant="body2" color="text.secondary">{formatCompact(row.total_tokens)}</Typography>,
    }
  })

  return (
    <Page
      breadcrumbTitle="Usage"
      breadcrumbParent={{
        title: 'Organizations',
        routeName: 'orgs',
        useOrgRouter: false,
      }}
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, pb: 4 }}>
          <Stack spacing={2.5}>
            <UsagePageHeader
              title="Usage"
              description="LLM calls, tokens, cost, latency, and sandbox compute for this organization."
              actions={(
                <UsageRangeControls
                  range={range}
                  from={from}
                  to={to}
                  onRangeChange={handleRangeChange}
                  onFromChange={value => { markFilterChange(); setFrom(value); setRange(null) }}
                  onToChange={value => { markFilterChange(); setTo(value); setRange(null) }}
                />
              )}
            />

            {usage.isError && (
              <Alert severity="error">
                Failed to load organization usage.
              </Alert>
            )}

            <Paper variant="outlined" sx={{ px: 1.5, pt: 1.5, pb: 1, borderRadius: 2 }}>
              <Button
                fullWidth
                variant="text"
                color="inherit"
                startIcon={<SlidersHorizontal size={16} />}
                onClick={() => setFiltersOpen(open => !open)}
                aria-expanded={filtersOpen}
                sx={{ display: { xs: 'flex', sm: 'none' }, justifyContent: 'flex-start', textTransform: 'none', mb: filtersOpen ? 1.25 : 0 }}
              >
                {activeFilters > 0 ? `Filters · ${activeFilters} active` : 'Filters'}
              </Button>
              <Box
                sx={{
                  display: { xs: filtersOpen ? 'grid' : 'none', sm: 'grid' },
                  gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', lg: 'repeat(4, minmax(0, 1fr))' },
                  gap: 1.25,
                }}
              >
                <FilterAutocomplete
                  label="User"
                  options={userOptions}
                  value={selectedUser}
                  loading={isScopedLoading('filters')}
                  onChange={option => { markFilterChange(); setUserId(option?.id || '') }}
                />
                <FilterAutocomplete
                  label="Project"
                  options={projectOptions}
                  value={selectedProject}
                  loading={isScopedLoading('filters')}
                  onChange={option => { markFilterChange(); setProjectId(option?.id || '') }}
                />
                <FilterAutocomplete
                  label="Task"
                  options={taskOptions}
                  value={selectedTask}
                  loading={isScopedLoading('filters')}
                  onChange={option => { markFilterChange(); setTaskId(option?.id || '') }}
                />
                <FilterAutocomplete
                  label="Agent"
                  options={appOptions}
                  value={selectedApp}
                  loading={isScopedLoading('filters')}
                  onChange={option => { markFilterChange(); setAppId(option?.id || '') }}
                />
                <TextField size="small" label="Session ID" value={sessionIdInput} onChange={e => { markFilterChange(); setSessionIdInput(e.target.value) }} />
                <FilterAutocomplete
                  label="Provider"
                  options={providerOptions}
                  value={selectedProvider}
                  loading={isScopedLoading('filters')}
                  onChange={option => {
                    markFilterChange()
                    setProvider(option?.id || '')
                    setModel('')
                  }}
                />
                <FilterAutocomplete
                  label="Model"
                  options={filteredModelOptions}
                  value={selectedModel}
                  loading={isScopedLoading('filters')}
                  onChange={option => {
                    markFilterChange()
                    setProvider(option?.provider || '')
                    setModel(option?.model || '')
                  }}
                />
                <Button size="small" variant="outlined" startIcon={<X size={16} />} onClick={clearFilters} sx={{ minHeight: 40 }}>
                  Clear
                </Button>
              </Box>
              <Box sx={{ height: 3, mt: 1 }}>
                {isScopedLoading('filters') && (
                  <LinearProgress sx={{ borderRadius: 1, height: 3 }} />
                )}
              </Box>
            </Paper>

            {usage.isLoading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress />
              </Box>
            ) : (
              <>
                <OverviewStatStrip columns={6}>
                  <OverviewStat emphasis primary label="LLM credits debited" value={formatCost(helixCredits)} detail={`≈ ${formatCost(rawTokenCost)} at API rates`} />
                  <OverviewStat
                    emphasis
                    label="Sandbox compute"
                    value={compute ? formatCost(computeTotals.total) : '—'}
                    detail={compute ? (compute.billing_enabled ? 'Desktops and containers' : 'Historical, billing off') : 'No compute in this range'}
                  />
                  <OverviewStat emphasis label="Processed tokens" value={formatCompact(totals.total)} detail={`${formatNumber(totals.requests)} LLM calls`} />
                  <OverviewStat emphasis label="Cached input" value={formatPercent(cachedShare)} detail={`${formatCompact(totals.cacheRead)} from cache`} />
                  <OverviewStat emphasis label="Cache savings" value={formatCost(cacheSavings)} detail={rawTokenCost > 0 ? `${(cacheSavings / rawTokenCost).toFixed(1)}× API-rate cost` : 'vs full input rates'} />
                  <OverviewStat
                    emphasis
                    label="Subscription savings"
                    value={formatCost(subscriptionSavings)}
                    detail={rawTokenCost > 0 ? `${formatPercent(subscriptionSavings / rawTokenCost)} of API-rate cost` : 'No subscription usage'}
                  />
                </OverviewStatStrip>

                <UsageGroup
                  title="LLM tokens and spend"
                  description="What each provider and model processed, and its cost at API rates. Every filter applies."
                >
                  <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(260px, 0.7fr) minmax(0, 1.6fr)' }, gap: 2 }}>
                    <Paper variant="outlined" sx={{ p: 2, borderRadius: 2 }}>
                      <Stack spacing={2}>
                        <Box>
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            {overviewMetric === 'cost' ? 'By provider, at API rates' : 'By provider, processed tokens'}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {overviewMetric === 'cost'
                              ? `${formatCost(rawTokenCost)} API-rate equivalent, not the amount debited; subscription usage is not charged per token.`
                              : `${formatCompact(totals.total)} tokens across ${formatNumber(totals.requests)} LLM calls.`}
                          </Typography>
                        </Box>
                        {providerRows.length ? providerRows.map((row, index) => (
                          <ProviderSummaryRow
                            key={row.id || row.provider || index}
                            provider={row}
                            metric={overviewMetric}
                            total={providerTotal}
                            color={providerColor(row.provider || '', index, lightTheme.isLight)}
                          />
                        )) : (
                          <Typography variant="body2" color="text.secondary">No provider activity in this window.</Typography>
                        )}
                      </Stack>
                    </Paper>
                    <ShadcnAreaChart
                      title={overviewMetric === 'cost' ? 'Daily API-rate cost' : 'Daily processed tokens'}
                      headline={overviewMetric === 'cost' ? `${formatCost(rawTokenCost)} in range` : `${formatCompact(totals.total)} in range`}
                      action={(
                        <ToggleButtonGroup
                          value={overviewMetric}
                          exclusive
                          size="small"
                          onChange={(_, next: OverviewMetric | null) => next && setOverviewMetric(next)}
                        >
                          <ToggleButton value="cost">Cost</ToggleButton>
                          <ToggleButton value="tokens">Tokens</ToggleButton>
                        </ToggleButtonGroup>
                      )}
                      data={providerChartData}
                      series={providerChartSeries}
                      valueFormatter={overviewMetric === 'cost' ? formatCost : formatCompact}
                      variant="bar"
                      cardHeight="auto"
                    />
                  </Box>

                  <OverviewStatStrip columns={4}>
                    <OverviewStat label="Tokens per call" value={formatCompact(totals.requests > 0 ? totals.total / totals.requests : 0)} detail={`Across ${formatNumber(totals.requests)} LLM calls`} />
                    <OverviewStat label="Tokens per active day" value={formatCompact(averageTokens)} detail={`${activeDays} ${activeDays === 1 ? 'day' : 'days'} with LLM calls`} />
                    <OverviewStat label="API-rate token cost" value={formatCost(rawTokenCost)} detail="At model list prices" />
                    <OverviewStat label="Cached input tokens" value={formatCompact(totals.cacheRead)} detail={`${formatPercent(cachedShare)} of observed input`} />
                  </OverviewStatStrip>
                  <Typography variant="caption" color="text.secondary">
                    Savings use current model API rates. Helix credits include only metered token charges debited by Helix; provider subscriptions and external API keys are excluded.
                  </Typography>

                  <UsagePanel>
                    <UsageSection title="Model breakdown">
                      <TableScroll><SimpleTable authenticated fields={modelOverviewFields} data={modelOverviewRows} compact /></TableScroll>
                    </UsageSection>
                  </UsagePanel>

                  <ShadcnAreaChart
                    title="Tokens over time"
                    headline={`${formatCompact(totals.total)} tokens`}
                    data={tokenChartData}
                    series={TOKEN_SERIES}
                    valueFormatter={formatCompact}
                    variant="bar"
                    cardHeight="auto"
                  />

                  <UsageChartGrid>
                    <ShadcnAreaChart
                      title={latencyMetric === 'per-request' ? 'Latency by provider' : 'Latency per 1k output tokens'}
                      headline={latencyHeadline}
                      action={(
                        <ToggleButtonGroup
                          value={latencyMetric}
                          exclusive
                          size="small"
                          onChange={(_, next: LatencyMetric | null) => next && setLatencyMetric(next)}
                        >
                          <ToggleButton value="per-request">Per call</ToggleButton>
                          <ToggleButton value="per-1k-output">Per 1k out</ToggleButton>
                        </ToggleButtonGroup>
                      )}
                      data={latencyChartData}
                      series={latencyChartSeries}
                      valueFormatter={formatMs}
                      stacked={false}
                      variant="line"
                      connectNulls
                      cardHeight="auto"
                      chartHeight={200}
                    />
                    <ShadcnAreaChart
                      title="Tool call error rate by provider"
                      headline={toolCallErrorHeadline}
                      data={toolCallErrorChartData}
                      series={providerChartSeries}
                      valueFormatter={value => (value > 0 && value < 0.01 ? `${(value * 100).toFixed(2)}%` : formatPercent(value))}
                      stacked={false}
                      zeroIsData
                      variant="line"
                      connectNulls
                      cardHeight="auto"
                      chartHeight={200}
                    />
                    <ShadcnAreaChart
                      title="Cache hit ratio by agent runtime"
                      headline={`${formatPercent(cacheHitRatio)} overall`}
                      data={agentCacheChartData}
                      series={agentCacheChartSeries}
                      valueFormatter={value => formatPercent(value)}
                      stacked={false}
                      zeroIsData
                      variant="line"
                      yDomain={[0, 1]}
                      cardHeight="auto"
                      chartHeight={200}
                    />
                    <ShadcnAreaChart
                      title="Model usage over time"
                      headline={`${modelSeries.length} ${modelSeries.length === 1 ? 'model' : 'models'}`}
                      data={modelChartData}
                      series={modelChartSeries}
                      valueFormatter={formatCompact}
                      variant="bar"
                      cardHeight="auto"
                      chartHeight={200}
                    />
                  </UsageChartGrid>
                </UsageGroup>

                {compute && (
                  <UsageGroup
                    title="Sandbox compute"
                    description={`${compute.billing_enabled
                      ? 'Credits debited for running desktops and containers.'
                      : 'Compute billing is disabled — this is historical spend and nothing new is accruing.'} The date range, project and task filters apply; model, provider, session and user filters describe tokens only.`}
                  >
                    <OverviewStatStrip columns={4}>
                      <OverviewStat
                        label="Desktops"
                        value={formatCost(computeTotals.desktop)}
                        detail={computeTotals.total > 0 ? `${formatPercent(computeTotals.desktop / computeTotals.total)} of compute spend` : 'No desktop spend'}
                      />
                      <OverviewStat
                        label="Headless"
                        value={formatCost(computeTotals.headless)}
                        detail={computeTotals.total > 0 ? `${formatPercent(computeTotals.headless / computeTotals.total)} of compute spend` : 'No headless spend'}
                      />
                      <OverviewStat
                        label="Per active day"
                        value={formatCost(computeTotals.perActiveDay)}
                        detail={`${computeTotals.activeDays} ${computeTotals.activeDays === 1 ? 'day' : 'days'} with compute spend`}
                      />
                      <OverviewStat
                        label="Running now"
                        value={`${compute.running_sandboxes ?? 0}`}
                        detail="Sandboxes currently billing"
                      />
                    </OverviewStatStrip>

                    <ShadcnAreaChart
                      title="Daily compute spend"
                      headline={`${formatCost(computeTotals.total)} in range`}
                      data={computeChartData}
                      series={COMPUTE_SERIES}
                      valueFormatter={formatCost}
                      zeroIsData
                      variant="bar"
                      cardHeight="auto"
                    />

                    {computeSandboxRows.length > 0 && (
                      <UsagePanel>
                        <UsageSection title="Sandbox breakdown">
                          <TableScroll><SimpleTable authenticated fields={computeSandboxFields} data={computeSandboxRows} compact /></TableScroll>
                        </UsageSection>
                      </UsagePanel>
                    )}
                  </UsageGroup>
                )}

                <UsageGroup
                  title="Breakdowns"
                  description="Tokens, cost and latency attributed to projects, agents, tasks, sessions and users. Export any table as CSV or JSON."
                >
                  <ProjectModelChart data={projectModelChart.data} series={projectModelChart.series} />
                  <UsagePanel>
                    <UsageSection title="Projects" action={<ExportButtons filename={`org-${orgID}-projects-usage`} rows={usage.data?.export_projects || usage.data?.projects || []} />}>
                      <BreakdownTable empty="No projects in this range." loading={isScopedLoading('projects')} rows={tableRows.projects}>
                        <TableScroll><SimpleTable authenticated fields={baseFields} data={tableRows.projects} compact loading={isScopedLoading('projects')} /></TableScroll>
                      </BreakdownTable>
                      {(usage.data?.projects_total || 0) > projectRowsPerPage && (<TablePagination
                        component="div"
                        count={usage.data?.projects_total || 0}
                        page={projectPage}
                        rowsPerPage={projectRowsPerPage}
                        onPageChange={(_, nextPage) => {
                          setLoadingScope('projects')
                          setProjectPage(nextPage)
                        }}
                        onRowsPerPageChange={event => {
                          setLoadingScope('projects')
                          setProjectRowsPerPage(parseInt(event.target.value, 10))
                          setProjectPage(0)
                        }}
                        rowsPerPageOptions={[10, 25, 50, 100]}
                      />)}
                    </UsageSection>
                  </UsagePanel>

                  <UsagePanel>
                    <UsageSection title="Agent" action={<ExportButtons filename={`org-${orgID}-agents-usage`} rows={usage.data?.export_apps || usage.data?.apps || []} />}>
                      <BreakdownTable empty="No agents in this range." rows={tableRows.apps}>
                        <TableScroll><SimpleTable authenticated fields={baseFields} data={tableRows.apps} compact /></TableScroll>
                      </BreakdownTable>
                    </UsageSection>
                  </UsagePanel>

                  <UsagePanel>
                    <UsageSection title="Tasks" action={<ExportButtons filename={`org-${orgID}-tasks-usage`} rows={usage.data?.export_tasks || usage.data?.tasks || []} />}>
                      <BreakdownTable empty="No tasks in this range." loading={isScopedLoading('tasks')} rows={tableRows.tasks}>
                        <TableScroll><SimpleTable authenticated fields={baseFields} data={tableRows.tasks} compact loading={isScopedLoading('tasks')} /></TableScroll>
                      </BreakdownTable>
                      {(usage.data?.tasks_total || 0) > taskRowsPerPage && (<TablePagination
                        component="div"
                        count={usage.data?.tasks_total || 0}
                        page={taskPage}
                        rowsPerPage={taskRowsPerPage}
                        onPageChange={(_, nextPage) => {
                          setLoadingScope('tasks')
                          setTaskPage(nextPage)
                        }}
                        onRowsPerPageChange={event => {
                          setLoadingScope('tasks')
                          setTaskRowsPerPage(parseInt(event.target.value, 10))
                          setTaskPage(0)
                        }}
                        rowsPerPageOptions={[10, 25, 50, 100]}
                      />)}
                    </UsageSection>
                  </UsagePanel>

                  <UsagePanel>
                    <UsageSection title="Sessions" action={<ExportButtons filename={`org-${orgID}-sessions-usage`} rows={usage.data?.export_sessions || usage.data?.sessions || []} />}>
                      <BreakdownTable empty="No sessions in this range." loading={isScopedLoading('sessions')} rows={tableRows.sessions}>
                        <TableScroll><SimpleTable authenticated fields={sessionFields} data={tableRows.sessions} compact loading={isScopedLoading('sessions')} /></TableScroll>
                      </BreakdownTable>
                      {(usage.data?.sessions_total || 0) > sessionRowsPerPage && (<TablePagination
                        component="div"
                        count={usage.data?.sessions_total || 0}
                        page={sessionPage}
                        rowsPerPage={sessionRowsPerPage}
                        onPageChange={(_, nextPage) => {
                          setLoadingScope('sessions')
                          setSessionPage(nextPage)
                        }}
                        onRowsPerPageChange={event => {
                          setLoadingScope('sessions')
                          setSessionRowsPerPage(parseInt(event.target.value, 10))
                          setSessionPage(0)
                        }}
                        rowsPerPageOptions={[10, 25, 50, 100]}
                      />)}
                    </UsageSection>
                  </UsagePanel>

                  <UsagePanel>
                    <UsageSection
                      title="Users"
                      action={<ExportButtons filename={`org-${orgID}-users-usage`} rows={usage.data?.export_users || usage.data?.users || []} />}
                    >
                      <TextField
                        size="small"
                        placeholder="Search username or email"
                        value={userSearchInput}
                        onChange={e => {
                          setLoadingScope('users')
                          setUserSearchInput(e.target.value)
                          setUserPage(0)
                        }}
                        sx={{ mb: 1.5, width: '100%', maxWidth: 360 }}
                        InputProps={{
                          startAdornment: (
                            <InputAdornment position="start">
                              <Search size={16} />
                            </InputAdornment>
                          ),
                        }}
                      />
                      <BreakdownTable empty="No users in this range." loading={isScopedLoading('users')} rows={tableRows.users}>
                        <TableScroll><SimpleTable authenticated fields={baseFields} data={tableRows.users} compact loading={isScopedLoading('users')} /></TableScroll>
                      </BreakdownTable>
                      {(usage.data?.users_total || 0) > userRowsPerPage && (<TablePagination
                        component="div"
                        count={usage.data?.users_total || 0}
                        page={userPage}
                        rowsPerPage={userRowsPerPage}
                        onPageChange={(_, nextPage) => {
                          setLoadingScope('users')
                          setUserPage(nextPage)
                        }}
                        onRowsPerPageChange={event => {
                          setLoadingScope('users')
                          setUserRowsPerPage(parseInt(event.target.value, 10))
                          setUserPage(0)
                        }}
                        rowsPerPageOptions={[10, 25, 50, 100]}
                      />)}
                    </UsageSection>
                  </UsagePanel>
                </UsageGroup>
              </>
            )}
          </Stack>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgUsage
