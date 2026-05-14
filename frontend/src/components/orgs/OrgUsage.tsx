import React, { FC, useMemo, useState } from 'react'
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
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { TooltipContentProps } from 'recharts'
import { Download, Search } from 'lucide-react'

import type { TypesAggregatedUsageMetric, TypesUsageBreakdownRow, TypesUsageModelTimeSeries } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import Page from '../system/Page'
import SimpleTable, { ITableField } from '../widgets/SimpleTable'
import ShadcnAreaChart, { ShadcnSeries } from '../usage/ShadcnAreaChart'
import { useGetOrgUsage } from '../../services/orgService'

type RangeKey = '7d' | '30d' | '90d'

const TOKEN_SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Input', color: '#2563eb' },
  { key: 'cacheRead', label: 'Cache read', color: '#16a34a' },
  { key: 'cacheWrite', label: 'Cache write', color: '#d97706' },
  { key: 'output', label: 'Output', color: '#9333ea' },
]

const MODEL_COLORS = ['#2563eb', '#16a34a', '#d97706', '#9333ea', '#0891b2', '#e11d48']

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

const sumMetrics = (metrics: TypesAggregatedUsageMetric[] = []) => metrics.reduce((acc, metric) => ({
  prompt: acc.prompt + (metric.prompt_tokens ?? 0),
  completion: acc.completion + (metric.completion_tokens ?? 0),
  cacheRead: acc.cacheRead + (metric.cache_read_tokens ?? 0),
  cacheWrite: acc.cacheWrite + (metric.cache_write_tokens ?? 0),
  total: acc.total + (metric.total_tokens ?? 0),
  cost: acc.cost + (metric.total_cost ?? 0),
  requests: acc.requests + (metric.total_requests ?? 0),
}), { prompt: 0, completion: 0, cacheRead: 0, cacheWrite: 0, total: 0, cost: 0, requests: 0 })

const csvEscape = (value: string | number | undefined) => `"${String(value ?? '').replace(/"/g, '""')}"`

const exportRows = (filename: string, rows: TypesUsageBreakdownRow[]) => {
  const headers = ['name', 'email', 'username', 'provider', 'model', 'requests', 'input_tokens', 'output_tokens', 'cache_read_tokens', 'cache_write_tokens', 'total_tokens', 'total_cost', 'latency_ms']
  const lines = [
    headers.join(','),
    ...rows.map(row => [
      row.name,
      row.email,
      row.username,
      row.provider,
      row.model,
      row.total_requests,
      row.prompt_tokens,
      row.completion_tokens,
      row.cache_read_tokens,
      row.cache_write_tokens,
      row.total_tokens,
      row.total_cost,
      row.latency_ms,
    ].map(csvEscape).join(',')),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

const UsageCard: FC<{ label: string; value: string; sublabel?: string }> = ({ label, value, sublabel }) => (
  <Paper
    variant="outlined"
    sx={{
      p: 2,
      borderRadius: 2,
      bgcolor: 'rgba(0,0,0,0.02)',
      borderColor: 'rgba(255,255,255,0.08)',
      minHeight: 96,
    }}
  >
    <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase', letterSpacing: '0.04em' }}>
      {label}
    </Typography>
    <Typography variant="h5" sx={{ mt: 0.75, fontVariantNumeric: 'tabular-nums' }}>
      {value}
    </Typography>
    {sublabel && (
      <Typography variant="caption" color="text.secondary">
        {sublabel}
      </Typography>
    )}
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

const ExportButton: FC<{ filename: string; rows: TypesUsageBreakdownRow[] }> = ({ filename, rows }) => (
  <Button
    size="small"
    variant="outlined"
    startIcon={<Download size={16} />}
    onClick={() => exportRows(filename, rows)}
    disabled={!rows.length}
  >
    CSV
  </Button>
)

const baseFields: ITableField[] = [
  { name: 'name', title: 'Name' },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'input', title: 'Input', numeric: true },
  { name: 'output', title: 'Output', numeric: true },
  { name: 'cache', title: 'Cache', numeric: true },
  { name: 'total', title: 'Total', numeric: true },
  { name: 'cost', title: 'Cost', numeric: true },
  { name: 'latency', title: 'Latency', numeric: true },
]

const modelFields: ITableField[] = [
  { name: 'name', title: 'Model' },
  { name: 'provider', title: 'Provider' },
  ...baseFields.slice(1),
]

const rowToTable = (row: TypesUsageBreakdownRow, withProvider = false) => ({
  id: row.id,
  _data: row,
  name: (
    <Box>
      <Typography variant="body2" sx={{ fontWeight: 600 }}>{row.name || row.id || 'Unknown'}</Typography>
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
  requests: <Typography variant="body2">{formatNumber(row.total_requests)}</Typography>,
  input: <Typography variant="body2">{formatNumber(row.prompt_tokens)}</Typography>,
  output: <Typography variant="body2">{formatNumber(row.completion_tokens)}</Typography>,
  cache: <Typography variant="body2">{formatNumber((row.cache_read_tokens ?? 0) + (row.cache_write_tokens ?? 0))}</Typography>,
  total: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatNumber(row.total_tokens)}</Typography>,
  cost: <Typography variant="body2">{formatCost(row.total_cost)}</Typography>,
  latency: <Typography variant="body2">{formatMs(row.latency_ms)}</Typography>,
})

const LatencyTooltip: FC<TooltipContentProps<number, string>> = ({ active, payload, label }) => {
  if (!active || !payload?.length) return null
  return (
    <Box sx={{ bgcolor: 'rgba(10,10,15,0.95)', border: '1px solid rgba(255,255,255,0.12)', borderRadius: 2, px: 1.5, py: 1 }}>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
        {label ? new Date(label as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : ''}
      </Typography>
      <Typography variant="body2">{formatMs(payload[0]?.value as number)}</Typography>
    </Box>
  )
}

const LatencyChart: FC<{ data: Array<{ date: string; latency: number }> }> = ({ data }) => {
  const hasData = data.some(row => row.latency > 0)
  return (
    <Box sx={{ height: 300, bgcolor: 'rgba(0,0,0,0.2)', borderRadius: 2, p: 2 }}>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1, fontWeight: 500, letterSpacing: '0.04em' }}>
        LATENCY
      </Typography>
      {hasData ? (
        <ResponsiveContainer width="100%" height={230}>
          <LineChart data={data} margin={{ top: 10, right: 12, left: 16, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="rgba(255,255,255,0.08)" />
            <XAxis
              dataKey="date"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              tick={{ fill: '#94A3B8', fontSize: 12 }}
              tickFormatter={(v: string) => new Date(v).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
            />
            <YAxis tickLine={false} axisLine={false} tickMargin={10} width={80} tick={{ fill: '#94A3B8', fontSize: 12 }} tickFormatter={(v: number) => `${Math.round(v)}ms`} />
            <Tooltip content={<LatencyTooltip />} cursor={{ stroke: 'rgba(255,255,255,0.2)' }} />
            <Line type="monotone" dataKey="latency" stroke="#14b8a6" strokeWidth={2} dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      ) : (
        <Box sx={{ height: 230, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Typography variant="body2" color="text.secondary">No latency data</Typography>
        </Box>
      )}
    </Box>
  )
}

const buildModelChart = (series: TypesUsageModelTimeSeries[] = []) => {
  const dates = new Map<string, Record<string, number | string>>()
  series.slice(0, MODEL_COLORS.length).forEach(model => {
    model.metrics?.forEach(metric => {
      if (!metric.date) return
      const entry = dates.get(metric.date) ?? { date: metric.date }
      entry[model.id || model.model || 'model'] = metric.total_tokens ?? 0
      dates.set(metric.date, entry)
    })
  })
  return Array.from(dates.values()).sort((a, b) => String(a.date).localeCompare(String(b.date)))
}

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
    .slice(0, MODEL_COLORS.length)
    .map((model, index) => ({
      ...model,
      color: MODEL_COLORS[index],
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
            <Tooltip content={<ProjectModelTooltip />} cursor={{ fill: 'rgba(255,255,255,0.04)' }} />
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
  const orgID = router.params.org_id as string
  const today = toDateInput(new Date())
  const [range, setRange] = useState<RangeKey | null>('7d')
  const [from, setFrom] = useState(rangeFrom(7))
  const [to, setTo] = useState(today)
  const [userSearch, setUserSearch] = useState('')
  const [userPage, setUserPage] = useState(0)
  const [userRowsPerPage, setUserRowsPerPage] = useState(10)

  const fromRFC = useMemo(() => toRFC3339(from), [from])
  const toRFC = useMemo(() => toRFC3339(to, true), [to])

  const usage = useGetOrgUsage(orgID, {
    from: fromRFC,
    to: toRFC,
    userSearch,
    userLimit: userRowsPerPage,
    userOffset: userPage * userRowsPerPage,
    enabled: Boolean(orgID),
  })

  const metrics = usage.data?.metrics || []
  const totals = useMemo(() => sumMetrics(metrics), [metrics])
  const tokenChartData = useMemo(() => metrics.map(metric => {
    const cacheRead = metric.cache_read_tokens ?? 0
    const cacheWrite = metric.cache_write_tokens ?? 0
    return {
      date: metric.date || '',
      input: Math.max((metric.prompt_tokens ?? 0) - cacheRead - cacheWrite, 0),
      output: metric.completion_tokens ?? 0,
      cacheRead,
      cacheWrite,
    }
  }), [metrics])

  const modelSeries = (usage.data?.model_time_series || []).slice(0, MODEL_COLORS.length)
  const modelChartData = useMemo(() => buildModelChart(modelSeries), [modelSeries])
  const modelChartSeries: ShadcnSeries[] = modelSeries.map((model, index) => ({
    key: model.id || model.model || `model-${index}`,
    label: model.name || model.model || `Model ${index + 1}`,
    color: MODEL_COLORS[index],
  }))

  const latencyData = useMemo(() => metrics.map(metric => ({
    date: metric.date || '',
    latency: metric.latency_ms ?? 0,
  })), [metrics])
  const projectModelChart = useMemo(() => {
    return buildProjectModelChart(usage.data?.project_models || [], usage.data?.projects || [])
  }, [usage.data?.project_models, usage.data?.projects])

  const handleRangeChange = (_: React.MouseEvent<HTMLElement>, next: RangeKey | null) => {
    if (!next) return
    setRange(next)
    const days = next === '7d' ? 7 : next === '30d' ? 30 : 90
    setFrom(rangeFrom(days))
    setTo(today)
    setUserPage(0)
  }

  const tableRows = {
    projects: (usage.data?.projects || []).map(row => rowToTable(row)),
    tasks: (usage.data?.tasks || []).map(row => rowToTable(row)),
    models: (usage.data?.models || []).map(row => rowToTable(row, true)),
    users: (usage.data?.users || []).map(row => rowToTable(row)),
  }

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
                  LLM calls, tokens, cost, and latency for this organization.
                </Typography>
              </Box>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
                <ToggleButtonGroup value={range} exclusive size="small" onChange={handleRangeChange}>
                  <ToggleButton value="7d">7D</ToggleButton>
                  <ToggleButton value="30d">30D</ToggleButton>
                  <ToggleButton value="90d">90D</ToggleButton>
                </ToggleButtonGroup>
                <TextField size="small" label="From" type="date" value={from} onChange={e => { setFrom(e.target.value); setRange(null); setUserPage(0) }} InputLabelProps={{ shrink: true }} />
                <TextField size="small" label="To" type="date" value={to} onChange={e => { setTo(e.target.value); setRange(null); setUserPage(0) }} InputLabelProps={{ shrink: true }} />
              </Stack>
            </Stack>

            {usage.isError && (
              <Alert severity="error">
                Failed to load organization usage.
              </Alert>
            )}

            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, 1fr)', lg: 'repeat(4, 1fr)' }, gap: 1.5 }}>
              <UsageCard label="LLM calls" value={formatNumber(totals.requests)} />
              <UsageCard label="Total tokens" value={formatCompact(totals.total)} sublabel={`${formatCompact(totals.prompt)} input / ${formatCompact(totals.completion)} output`} />
              <UsageCard label="Cache tokens" value={formatCompact(totals.cacheRead + totals.cacheWrite)} sublabel={`${formatCompact(totals.cacheRead)} read / ${formatCompact(totals.cacheWrite)} write`} />
              <UsageCard label="Estimated cost" value={formatCost(totals.cost)} />
            </Box>

            {usage.isLoading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress />
              </Box>
            ) : (
              <>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1.35fr) minmax(0, 1fr)' }, gap: 2 }}>
                  <ShadcnAreaChart
                    title="TOKENS OVER TIME"
                    headline={formatCompact(totals.total)}
                    data={tokenChartData}
                    series={TOKEN_SERIES}
                    valueFormatter={formatCompact}
                  />
                  <LatencyChart data={latencyData} />
                </Box>

                <ShadcnAreaChart
                  title="TOP MODELS OVER TIME"
                  headline={`${modelSeries.length} models`}
                  data={modelChartData}
                  series={modelChartSeries}
                  valueFormatter={formatCompact}
                  stacked={false}
                />

                <ProjectModelChart data={projectModelChart.data} series={projectModelChart.series} />

                <Stack spacing={3} sx={{ width: '100%' }}>
                  <Section title="Projects" action={<ExportButton filename={`org-${orgID}-projects-usage.csv`} rows={usage.data?.projects || []} />}>
                    <SimpleTable authenticated fields={baseFields} data={tableRows.projects} compact />
                  </Section>

                  <Section title="Top Tasks By Model" action={<ExportButton filename={`org-${orgID}-tasks-usage.csv`} rows={usage.data?.tasks || []} />}>
                    <SimpleTable authenticated fields={baseFields} data={tableRows.tasks} compact />
                  </Section>

                  <Section title="Models" action={<ExportButton filename={`org-${orgID}-models-usage.csv`} rows={usage.data?.models || []} />}>
                    <SimpleTable authenticated fields={modelFields} data={tableRows.models} compact />
                  </Section>

                  <Section
                    title="Users"
                    action={<ExportButton filename={`org-${orgID}-users-usage.csv`} rows={usage.data?.users || []} />}
                  >
                    <TextField
                      size="small"
                      placeholder="Search username or email"
                      value={userSearch}
                      onChange={e => { setUserSearch(e.target.value); setUserPage(0) }}
                      sx={{ mb: 1.5, width: '100%', maxWidth: 360 }}
                      InputProps={{
                        startAdornment: (
                          <InputAdornment position="start">
                            <Search size={16} />
                          </InputAdornment>
                        ),
                      }}
                    />
                    <SimpleTable authenticated fields={baseFields} data={tableRows.users} compact loading={usage.isFetching && !usage.isLoading} />
                    <TablePagination
                      component="div"
                      count={usage.data?.users_total || 0}
                      page={userPage}
                      rowsPerPage={userRowsPerPage}
                      onPageChange={(_, nextPage) => setUserPage(nextPage)}
                      onRowsPerPageChange={event => {
                        setUserRowsPerPage(parseInt(event.target.value, 10))
                        setUserPage(0)
                      }}
                      rowsPerPageOptions={[10, 25, 50, 100]}
                    />
                  </Section>
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
