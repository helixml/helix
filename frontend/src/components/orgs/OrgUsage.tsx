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
import InputAdornment from '@mui/material/InputAdornment'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import Autocomplete from '@mui/material/Autocomplete'
import LinearProgress from '@mui/material/LinearProgress'
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { TooltipContentProps } from 'recharts'
import { Cloud, Download, Search, X } from 'lucide-react'

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
  getCacheHitRatio,
  getCacheUsageSeriesKey,
  getTotalInputTokens,
  getUncachedInputTokens,
} from '../../utils/usageMetrics'

type RangeKey = '7d' | '30d' | '90d'
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

const toDateInput = (date: Date) => date.toISOString().slice(0, 10)

const rangeFrom = (days: number) => {
  const from = new Date()
  from.setDate(from.getDate() - (days - 1))
  return toDateInput(from)
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
  return toDateInput(parsed)
}

const currentSearchParams = () => new URLSearchParams(window.location.search)

const formatCompact = (value?: number) => {
  const n = value ?? 0
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return n.toLocaleString()
}

const formatNumber = (value?: number) => (value ?? 0).toLocaleString()

const formatCost = (value?: number) => {
  const n = value ?? 0
  if (n === 0) return '$0.00'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

const formatMs = (value?: number) => `${Math.round(value ?? 0).toLocaleString()} ms`

const formatPercent = (ratio: number | null) => ratio === null ? '—' : `${(ratio * 100).toFixed(1)}%`

const formatDateTime = (value?: string) => value ? new Date(value).toLocaleString() : '-'

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

const OverviewStat: FC<{ label: string; value: string; detail: string }> = ({ label, value, detail }) => (
  <Box sx={{ bgcolor: 'background.default', px: 2, py: 1.5, minWidth: 0 }}>
    <Typography variant="caption" color="text.secondary">{label}</Typography>
    <Typography sx={{ fontSize: '1.1rem', lineHeight: 1.45, fontVariantNumeric: 'tabular-nums' }}>{value}</Typography>
    <Typography variant="caption" color="text.secondary" noWrap>{detail}</Typography>
  </Box>
)

const ProviderSummaryRow: FC<{
  provider: TypesUsageBreakdownRow
  metric: OverviewMetric
  total: number
  color: string
}> = ({ provider, metric, total, color }) => {
  const value = metric === 'cost' ? (provider.total_cost ?? 0) : (provider.total_tokens ?? 0)
  const share = total > 0 ? value / total : 0
  return (
    <Box>
      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
          <UsageProviderIcon provider={provider.provider} />
          <Typography variant="body2" noWrap>{provider.name || provider.provider || 'Unknown'}</Typography>
        </Stack>
        <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums' }}>
          {metric === 'cost' ? formatCost(value) : formatCompact(value)}
        </Typography>
      </Stack>
      <Box sx={{ height: 4, bgcolor: 'action.hover', borderRadius: 4, overflow: 'hidden', mt: 0.75 }}>
        <Box sx={{ width: `${Math.min(share * 100, 100)}%`, height: '100%', bgcolor: color }} />
      </Box>
      <Typography variant="caption" color="text.secondary">
        {formatPercent(share)} of {metric} · {formatCompact(provider.total_tokens)} tokens
      </Typography>
    </Box>
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

const Section: FC<{ title: string; action?: React.ReactNode; children: React.ReactNode }> = ({ title, action, children }) => (
  <Box sx={{ width: '100%' }}>
    <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2} sx={{ mb: 1.5 }}>
      <Typography variant="h6" sx={{ fontSize: '1rem' }}>
        {title}
      </Typography>
      {action}
    </Stack>
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

const baseFields: ITableField[] = [
  { name: 'name', title: 'Name' },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'sessions', title: 'Sessions', numeric: true },
  { name: 'input', title: 'Input', numeric: true },
  { name: 'output', title: 'Output', numeric: true },
  { name: 'cache', title: 'Cache read', numeric: true },
  { name: 'cacheHit', title: 'Hit ratio', numeric: true },
  { name: 'total', title: 'Total', numeric: true },
  { name: 'cost', title: 'Cost', numeric: true },
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
  { name: 'users', title: 'Users', numeric: true },
  { name: 'projects', title: 'Projects', numeric: true },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'input', title: 'Input', numeric: true },
  { name: 'output', title: 'Output', numeric: true },
  { name: 'cache', title: 'Cache read', numeric: true },
  { name: 'cacheHit', title: 'Hit ratio', numeric: true },
  { name: 'total', title: 'Total', numeric: true },
  { name: 'cost', title: 'Cost', numeric: true },
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
          <Typography variant="body2" sx={{ fontWeight: 600, '&:hover': { textDecoration: 'underline' } }}>{row.name || row.id || 'Unknown'}</Typography>
        </a>
      ) : (
        <Typography variant="body2" sx={{ fontWeight: 600 }}>{row.name || row.id || 'Unknown'}</Typography>
      )}
      {(row.email || row.username) && (
        <Typography variant="caption" color="text.secondary">
          {[row.email, row.username].filter(Boolean).join(' · ')}
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
  input: <Typography variant="body2">{formatNumber(getTotalInputTokens(row))}</Typography>,
  output: <Typography variant="body2">{formatNumber(row.completion_tokens)}</Typography>,
  cache: <Typography variant="body2">{formatNumber(row.cache_read_tokens)}</Typography>,
  cacheHit: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatPercent(getCacheHitRatio(row))}</Typography>,
  total: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatNumber(row.total_tokens)}</Typography>,
  cost: <Typography variant="body2">{formatCost(row.total_cost)}</Typography>,
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
  if (!active || !payload?.length) return null
  return (
    <Box sx={{ bgcolor: 'rgba(10,10,15,0.95)', border: '1px solid rgba(255,255,255,0.12)', borderRadius: 2, px: 1.5, py: 1, minWidth: 220 }}>
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

const ProjectModelChart: FC<{
  data: Array<Record<string, number | string>>,
  series: Array<{ key: string; label: string; color: string }>,
}> = ({ data, series }) => {
  const hasData = data.some(row => series.some(item => Number(row[item.key] || 0) > 0))
  return (
    <Box sx={{ height: 340, bgcolor: 'rgba(0,0,0,0.2)', borderRadius: 2, p: 2 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1 }}>
        <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500, letterSpacing: '0.04em' }}>
          PROJECT TOKENS BY MODEL
        </Typography>
        <Typography variant="caption" color="text.secondary">
          Top projects, stacked by model
        </Typography>
      </Stack>
      {hasData ? (
        <ResponsiveContainer width="100%" height={270}>
          <BarChart data={data} margin={{ top: 8, right: 12, left: 16, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="rgba(255,255,255,0.08)" />
            <XAxis
              dataKey="project"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              interval={0}
              tick={{ fill: '#94A3B8', fontSize: 12 }}
            />
            <YAxis tickLine={false} axisLine={false} tickMargin={10} width={72} tick={{ fill: '#94A3B8', fontSize: 12 }} tickFormatter={formatCompact} />
            <Tooltip content={React.createElement(ProjectModelTooltip)} cursor={{ fill: 'rgba(255,255,255,0.04)' }} />
            <Legend wrapperStyle={{ fontSize: '0.75rem' }} />
            {series.map(item => (
              <Bar key={item.key} dataKey={item.key} name={item.label} stackId="tokens" fill={item.color} isAnimationActive={false} />
            ))}
          </BarChart>
        </ResponsiveContainer>
      ) : (
        <Box sx={{ height: 270, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Typography variant="body2" color="text.secondary">No project/model usage data</Typography>
        </Box>
      )}
    </Box>
  )
}

const OrgUsage: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const orgID = router.params.org_id as string
  const today = toDateInput(new Date())
  const initialParams = useMemo(() => currentSearchParams(), [])
  const [range, setRange] = useState<RangeKey | null>(() => (initialParams.has('from') || initialParams.has('to') ? null : '7d'))
  const [from, setFrom] = useState(() => fromURLDate(initialParams.get('from'), rangeFrom(7)))
  const [to, setTo] = useState(() => fromURLDate(initialParams.get('to'), today))
  const [userId, setUserId] = useState(() => initialParams.get('user_id') || '')
  const [projectId, setProjectId] = useState(() => initialParams.get('project_id') || '')
  const [taskId, setTaskId] = useState(() => initialParams.get('task_id') || '')
  const [appId, setAppId] = useState(() => initialParams.get('app_id') || '')
  const [sessionIdInput, setSessionIdInput] = useState(() => initialParams.get('session_id') || '')
  const [provider, setProvider] = useState(() => initialParams.get('provider') || '')
  const [model, setModel] = useState(() => initialParams.get('model') || '')
  const [userSearchInput, setUserSearchInput] = useState(() => initialParams.get('user_search') || '')
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

  const handleRangeChange = (_: React.MouseEvent<HTMLElement>, next: RangeKey | null) => {
    if (!next) return
    markFilterChange()
    setRange(next)
    const days = next === '7d' ? 7 : next === '30d' ? 30 : 90
    setFrom(rangeFrom(days))
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
            <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', md: 'center' }} spacing={2}>
              <Box>
                <Typography variant="h5" component="h2" sx={{ mb: 1 }}>Usage</Typography>
                <Typography variant="body2" color="text.secondary">
                  LLM calls, tokens, cost, latency, and sandbox compute for this organization.
                </Typography>
              </Box>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
                <ToggleButtonGroup value={range} exclusive size="small" onChange={handleRangeChange}>
                  <ToggleButton value="7d">7D</ToggleButton>
                  <ToggleButton value="30d">30D</ToggleButton>
                  <ToggleButton value="90d">90D</ToggleButton>
                </ToggleButtonGroup>
                <TextField size="small" label="From" type="date" value={from} onChange={e => { markFilterChange(); setFrom(e.target.value); setRange(null) }} InputLabelProps={{ shrink: true }} />
                <TextField size="small" label="To" type="date" value={to} onChange={e => { markFilterChange(); setTo(e.target.value); setRange(null) }} InputLabelProps={{ shrink: true }} />
              </Stack>
            </Stack>

            {usage.isError && (
              <Alert severity="error">
                Failed to load organization usage.
              </Alert>
            )}

            {usage.isLoading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress />
              </Box>
            ) : (
              <>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(260px, 0.72fr) minmax(0, 1.6fr)' }, gap: 3 }}>
                  <Stack spacing={2.25}>
                    <Box>
                      <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                        {overviewMetric === 'cost' ? 'Raw token cost' : 'Processed tokens'}
                      </Typography>
                      <Typography sx={{ mt: 0.25, fontSize: { xs: '2.25rem', md: '2.6rem' }, lineHeight: 1.1, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
                        {overviewMetric === 'cost' ? `${formatCost(rawTokenCost)}*` : formatCompact(totals.total)}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {overviewMetric === 'cost'
                          ? '* API-rate equivalent; subscription usage is not charged per token'
                          : `${formatNumber(totals.requests)} LLM calls in this window`}
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

                  <Box>
                    <Stack direction="row" justifyContent="flex-end" sx={{ mb: 1 }}>
                      <ToggleButtonGroup
                        value={overviewMetric}
                        exclusive
                        size="small"
                        onChange={(_, next: OverviewMetric | null) => next && setOverviewMetric(next)}
                      >
                        <ToggleButton value="cost">Cost</ToggleButton>
                        <ToggleButton value="tokens">Tokens</ToggleButton>
                      </ToggleButtonGroup>
                    </Stack>
                    <ShadcnAreaChart
                      title={`DAILY ${overviewMetric === 'cost' ? 'API-RATE COST' : 'PROCESSED TOKENS'}`}
                      headline={overviewMetric === 'cost' ? formatCost(rawTokenCost) : formatCompact(totals.total)}
                      data={providerChartData}
                      series={providerChartSeries}
                      valueFormatter={overviewMetric === 'cost' ? formatCost : formatCompact}
                      stacked={false}
                    />
                  </Box>
                </Box>

                <Box
                  sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(5, minmax(0, 1fr))' },
                    gap: '1px',
                    bgcolor: 'divider',
                    borderTop: '1px solid',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  <OverviewStat label="Processed tokens" value={formatCompact(totals.total)} detail={`${formatCompact(averageTokens)} per active day`} />
                  <OverviewStat
                    label="Subscription savings"
                    value={formatCost(subscriptionSavings)}
                    detail={rawTokenCost > 0 ? `${formatPercent(subscriptionSavings / rawTokenCost)} of API-rate cost avoided` : 'No subscription usage'}
                  />
                  <OverviewStat
                    label="Cache savings"
                    value={formatCost(cacheSavings)}
                    detail={rawTokenCost > 0 ? `${(cacheSavings / rawTokenCost).toFixed(1)}× the raw token cost` : 'vs full input rates'}
                  />
                  <OverviewStat label="Helix credits" value={formatCost(helixCredits)} detail="Actually debited for paid tokens" />
                  <OverviewStat label="Cached input" value={formatCompact(totals.cacheRead)} detail={`${formatPercent(cachedShare)} of observed input`} />
                </Box>

                <Section title="Model breakdown">
                  <SimpleTable authenticated fields={modelOverviewFields} data={modelOverviewRows} compact />
                </Section>
              </>
            )}

            <Paper variant="outlined" sx={{ px: 2, pt: 1.25, pb: 2, borderRadius: 2, bgcolor: 'rgba(0,0,0,0.02)', borderColor: 'rgba(255,255,255,0.08)' }}>
              <Box sx={{ height: 6, mb: 1.5 }}>
                {isScopedLoading('filters') && (
                  <LinearProgress sx={{ borderRadius: 1 }} />
                )}
              </Box>
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))', xl: 'repeat(4, minmax(0, 1fr))' }, gap: 1.5 }}>
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
            </Paper>

            <Typography variant="caption" color="text.secondary">
              Savings use current model API rates. Helix credits include only metered token charges debited by Helix; provider subscriptions and external API keys are excluded.
            </Typography>

            {!usage.isLoading && compute && (
              <Paper variant="outlined" sx={{ p: 0, overflow: 'hidden' }}>
                <Box sx={{ px: 2, pt: 2, pb: 1.5 }}>
                  <Typography variant="overline" color="text.secondary">Sandbox compute</Typography>
                  <Typography variant="h4" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                    {formatCost(computeTotals.total)}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {compute.billing_enabled
                      ? 'Credits debited for running desktops and containers'
                      : 'Compute billing is disabled — this is historical spend and nothing new is accruing'}
                  </Typography>
                </Box>

                <Box
                  sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(4, minmax(0, 1fr))' },
                    gap: '1px',
                    bgcolor: 'divider',
                    borderTop: '1px solid',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                  }}
                >
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
                </Box>

                <Box sx={{ p: 2 }}>
                  <ShadcnAreaChart
                    title="DAILY COMPUTE SPEND"
                    headline={formatCost(computeTotals.total)}
                    data={computeChartData}
                    series={COMPUTE_SERIES}
                    valueFormatter={formatCost}
                    zeroIsData
                  />
                </Box>

                {computeSandboxRows.length > 0 && (
                  <Box sx={{ px: 2, pb: 2 }}>
                    <Section title="Sandbox breakdown">
                      <SimpleTable authenticated fields={computeSandboxFields} data={computeSandboxRows} compact />
                    </Section>
                  </Box>
                )}

                <Box sx={{ px: 2, pb: 2 }}>
                  <Typography variant="caption" color="text.secondary">
                Compute spend answers the date range, project, and task filters — model, provider, session and user filters describe tokens, not containers.
                  </Typography>
                </Box>
              </Paper>
            )}

            {!usage.isLoading && (
              <>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1.35fr) minmax(0, 1fr)' }, gap: 2 }}>
                  <UsagePanel>
                    <ShadcnAreaChart
                      title="TOKENS OVER TIME"
                      headline={formatCompact(totals.total)}
                      data={tokenChartData}
                      series={TOKEN_SERIES}
                      valueFormatter={formatCompact}
                    />
                  </UsagePanel>
                  <UsagePanel>
                    <Stack direction="row" alignItems="center" justifyContent="flex-end" sx={{ mb: 1 }}>
                      <ToggleButtonGroup
                        value={latencyMetric}
                        exclusive
                        size="small"
                        onChange={(_, next: LatencyMetric | null) => next && setLatencyMetric(next)}
                      >
                        <ToggleButton value="per-request">Per request</ToggleButton>
                        <ToggleButton value="per-1k-output">Per 1k output</ToggleButton>
                      </ToggleButtonGroup>
                    </Stack>
                    <ShadcnAreaChart
                      title={latencyMetric === 'per-request' ? 'LATENCY BY PROVIDER' : 'LATENCY PER 1K OUTPUT TOKENS'}
                      headline={latencyHeadline}
                      data={latencyChartData}
                      series={latencyChartSeries}
                      valueFormatter={formatMs}
                      stacked={false}
                      variant="line"
                      connectNulls
                    />
                  </UsagePanel>
                </Box>

                <UsagePanel>
                  <ShadcnAreaChart
                    title="CACHE HIT RATIO BY AGENT HARNESS"
                    headline={formatPercent(cacheHitRatio)}
                    data={agentCacheChartData}
                    series={agentCacheChartSeries}
                    valueFormatter={value => formatPercent(value)}
                    stacked={false}
                    zeroIsData
                    variant="line"
                    yDomain={[0, 1]}
                  />
                </UsagePanel>

                <UsagePanel>
                  <ShadcnAreaChart
                    title="MODEL USAGE OVER TIME"
                    headline={`${modelSeries.length} models`}
                    data={modelChartData}
                    series={modelChartSeries}
                    valueFormatter={formatCompact}
                    stacked={false}
                  />
                </UsagePanel>

                <UsagePanel>
                  <ProjectModelChart data={projectModelChart.data} series={projectModelChart.series} />
                </UsagePanel>

                <Stack spacing={3} sx={{ width: '100%' }}>
                  <UsagePanel>
                    <Section title="Projects" action={<ExportButtons filename={`org-${orgID}-projects-usage`} rows={usage.data?.export_projects || usage.data?.projects || []} />}>
                      <SimpleTable authenticated fields={baseFields} data={tableRows.projects} compact loading={isScopedLoading('projects')} />
                      <TablePagination
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
                      />
                    </Section>
                  </UsagePanel>

                  <UsagePanel>
                    <Section title="Agent" action={<ExportButtons filename={`org-${orgID}-agents-usage`} rows={usage.data?.export_apps || usage.data?.apps || []} />}>
                      <SimpleTable authenticated fields={baseFields} data={tableRows.apps} compact />
                    </Section>
                  </UsagePanel>

                  <UsagePanel>
                    <Section title="Tasks" action={<ExportButtons filename={`org-${orgID}-tasks-usage`} rows={usage.data?.export_tasks || usage.data?.tasks || []} />}>
                      <SimpleTable authenticated fields={baseFields} data={tableRows.tasks} compact loading={isScopedLoading('tasks')} />
                      <TablePagination
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
                      />
                    </Section>
                  </UsagePanel>

                  <UsagePanel>
                    <Section title="Sessions" action={<ExportButtons filename={`org-${orgID}-sessions-usage`} rows={usage.data?.export_sessions || usage.data?.sessions || []} />}>
                      <SimpleTable authenticated fields={sessionFields} data={tableRows.sessions} compact loading={isScopedLoading('sessions')} />
                      <TablePagination
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
                      />
                    </Section>
                  </UsagePanel>

                  <UsagePanel>
                    <Section
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
                      <SimpleTable authenticated fields={baseFields} data={tableRows.users} compact loading={isScopedLoading('users')} />
                      <TablePagination
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
                      />
                    </Section>
                  </UsagePanel>
                </Stack>
              </>
            )}
          </Stack>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgUsage
