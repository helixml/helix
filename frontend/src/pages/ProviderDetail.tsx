import React, { FC, useEffect, useMemo, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import Typography from '@mui/material/Typography'
import { Activity, ArrowLeft, CheckCircle2, CircleAlert, Database, Gauge, Pencil, Server } from 'lucide-react'

import type { TypesAggregatedUsageMetric, TypesProviderEndpoint, TypesUsersAggregatedUsageMetric } from '../api/api'
import EditProviderEndpointDialog from '../components/dashboard/EditProviderEndpointDialog'
import ProviderEndpointIcon from '../components/providers/ProviderEndpointIcon'
import LMStudioModels from '../components/providers/LMStudioModels'
import Page from '../components/system/Page'
import ShadcnAreaChart, { ShadcnSeries } from '../components/usage/ShadcnAreaChart'
import SimpleTable, { ITableField } from '../components/widgets/SimpleTable'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import {
  useListProviders,
  useProviderDailyUsage,
  useProviderUsersDailyUsage,
} from '../services/providersService'
import {
  getAggregateCacheHitRatio,
  getCacheHitRatio,
  getTotalInputTokens,
  getUncachedInputTokens,
} from '../utils/usageMetrics'

type RangeKey = '7d' | '30d' | '90d'

const TOKEN_SERIES: ShadcnSeries[] = [
  { key: 'input', label: 'Uncached input', color: '#2563eb' },
  { key: 'cacheRead', label: 'Cache read', color: '#16a34a' },
  { key: 'cacheWrite', label: 'Cache write', color: '#d97706' },
  { key: 'output', label: 'Output', color: '#9333ea' },
]

const LATENCY_SERIES: ShadcnSeries[] = [
  { key: 'latency', label: 'End-to-end latency', color: '#0891b2' },
]

const THROUGHPUT_SERIES: ShadcnSeries[] = [
  { key: 'throughput', label: 'Output throughput', color: '#16a34a' },
]

const COST_SERIES: ShadcnSeries[] = [
  { key: 'cost', label: 'API-rate cost', color: '#d97757' },
]

const toDateInput = (date: Date) => date.toISOString().slice(0, 10)

const rangeFrom = (days: number) => {
  const from = new Date()
  from.setDate(from.getDate() - (days - 1))
  return toDateInput(from)
}

const fromURLDate = (value: string | null, fallback: string) => {
  if (!value) return fallback
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? fallback : toDateInput(parsed)
}

const toRFC3339 = (value: string, endOfDay = false) => {
  if (!value) return undefined
  return new Date(`${value}T${endOfDay ? '23:59:59.999' : '00:00:00.000'}Z`).toISOString()
}

const formatCompact = (value?: number) => {
  const number = value ?? 0
  if (number >= 1_000_000_000) return `${(number / 1_000_000_000).toFixed(1)}B`
  if (number >= 1_000_000) return `${(number / 1_000_000).toFixed(1)}M`
  if (number >= 1_000) return `${(number / 1_000).toFixed(1)}k`
  return number.toLocaleString()
}

const formatCost = (value?: number) => {
  const number = value ?? 0
  if (number === 0) return '$0.00'
  return number < 0.01 ? `$${number.toFixed(4)}` : `$${number.toFixed(2)}`
}

const formatMs = (value?: number) => `${Math.round(value ?? 0).toLocaleString()}\u00a0ms`
const formatRate = (value?: number) => `${(value ?? 0).toFixed(1)}\u00a0tok/s`
const formatPercent = (value: number | null) => value === null ? '—' : `${(value * 100).toFixed(1)}%`

const formatBytes = (value?: number) => {
  const bytes = value ?? 0
  if (bytes >= 1_000_000_000) return `${(bytes / 1_000_000_000).toFixed(1)} GB`
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toFixed(1)} MB`
  if (bytes >= 1_000) return `${(bytes / 1_000).toFixed(1)} kB`
  return `${bytes.toLocaleString()} B`
}

const aggregateMetrics = (metrics: TypesAggregatedUsageMetric[] = []) => {
  const totals = metrics.reduce((result, metric) => {
    const requests = metric.total_requests ?? 0
    return {
      input: result.input + getTotalInputTokens(metric),
      output: result.output + (metric.completion_tokens ?? 0),
      cacheRead: result.cacheRead + (metric.cache_read_tokens ?? 0),
      cacheWrite: result.cacheWrite + (metric.cache_write_tokens ?? 0),
      tokens: result.tokens + (metric.total_tokens ?? 0),
      requests: result.requests + requests,
      cost: result.cost + (metric.total_cost ?? 0),
      durationMs: result.durationMs + ((metric.latency_ms ?? 0) * requests),
      requestBytes: result.requestBytes + (metric.request_size_bytes ?? 0),
      responseBytes: result.responseBytes + (metric.response_size_bytes ?? 0),
    }
  }, {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
    tokens: 0,
    requests: 0,
    cost: 0,
    durationMs: 0,
    requestBytes: 0,
    responseBytes: 0,
  })

  return {
    ...totals,
    latencyMs: totals.requests > 0 ? totals.durationMs / totals.requests : 0,
    throughput: totals.durationMs > 0 ? totals.output / (totals.durationMs / 1000) : 0,
  }
}

const OverviewStat: FC<{ label: string; value: string; detail: string; icon: React.ReactNode; flat?: boolean }> = ({ label, value, detail, icon, flat = false }) => (
  <Box sx={{ bgcolor: flat ? 'background.paper' : 'background.default', px: 2, py: 1.75, minWidth: 0 }}>
    <Stack direction="row" alignItems="center" spacing={0.75} sx={{ color: 'text.secondary', mb: 0.5 }}>
      {icon}
      <Typography variant="caption">{label}</Typography>
    </Stack>
    <Typography sx={{ fontSize: '1.15rem', lineHeight: 1.45, fontVariantNumeric: 'tabular-nums' }}>{value}</Typography>
    <Typography variant="caption" color="text.secondary">{detail}</Typography>
  </Box>
)

const userFields: ITableField[] = [
  { name: 'name', title: 'User' },
  { name: 'requests', title: 'LLM calls', numeric: true },
  { name: 'tokens', title: 'Tokens', numeric: true },
  { name: 'share', title: 'Share', numeric: true },
  { name: 'cache', title: 'Cache hit', numeric: true },
  { name: 'latency', title: 'Avg latency', numeric: true },
  { name: 'throughput', title: 'Output tok/s', numeric: true },
  { name: 'cost', title: 'API-rate cost', numeric: true },
]

interface ProviderDetailProps {
  providerId?: string
  orgId?: string
  embedded?: boolean
  dateParamPrefix?: string
  onBack?: () => void
}

export default function ProviderDetail({
  providerId: providerIdProp,
  orgId: orgIdProp,
  embedded = false,
  dateParamPrefix = '',
  onBack,
}: ProviderDetailProps = {}) {
  const router = useRouter()
  const account = useAccount()
  const orgId = orgIdProp || router.params.org_id
  const providerId = providerIdProp || router.params.provider_id
  const today = toDateInput(new Date())
  const fromParam = `${dateParamPrefix}from`
  const toParam = `${dateParamPrefix}to`
  const initialParams = useMemo(() => new URLSearchParams(window.location.search), [dateParamPrefix])
  const [range, setRange] = useState<RangeKey | null>(() => (initialParams.has(fromParam) || initialParams.has(toParam) ? null : '7d'))
  const [from, setFrom] = useState(() => fromURLDate(initialParams.get(fromParam), rangeFrom(7)))
  const [to, setTo] = useState(() => fromURLDate(initialParams.get(toParam), today))
  const [editOpen, setEditOpen] = useState(false)

  const providers = useListProviders({
    loadModels: true,
    orgId: account.admin ? undefined : orgId,
    all: account.admin || undefined,
    enabled: Boolean(providerId),
  })
  const provider = providers.data?.find(item => item.id === providerId || item.name === providerId)
  const usageProviderId = provider
    ? (provider.id && provider.id !== '-' ? provider.id : provider.name || '')
    : ''
  const fromRFC = useMemo(() => toRFC3339(from), [from])
  const toRFC = useMemo(() => toRFC3339(to, true), [to])
  const usage = useProviderDailyUsage(usageProviderId, fromRFC, toRFC, Boolean(provider))
  const usersUsage = useProviderUsersDailyUsage(usageProviderId, fromRFC, toRFC, Boolean(provider) && Boolean(account.admin))

  useEffect(() => {
    const url = new URL(window.location.href)
    url.searchParams.set(fromParam, from)
    url.searchParams.set(toParam, to)
    window.history.replaceState({}, '', url.toString())
  }, [from, fromParam, to, toParam])

  const metrics = usage.data || []
  const totals = useMemo(() => aggregateMetrics(metrics), [metrics])
  const cacheHitRatio = useMemo(() => getAggregateCacheHitRatio(metrics), [metrics])
  const activeDays = metrics.filter(metric => (metric.total_tokens ?? 0) > 0).length
  const chartData = useMemo(() => metrics.map(metric => ({
    date: metric.date || '',
    input: getUncachedInputTokens(metric),
    cacheRead: metric.cache_read_tokens ?? 0,
    cacheWrite: metric.cache_write_tokens ?? 0,
    output: metric.completion_tokens ?? 0,
  })), [metrics])
  const latencyData = useMemo(() => metrics.map(metric => ({ date: metric.date || '', latency: metric.latency_ms ?? 0 })), [metrics])
  const throughputData = useMemo(() => metrics.map(metric => {
    const requests = metric.total_requests ?? 0
    const durationMs = (metric.latency_ms ?? 0) * requests
    return {
      date: metric.date || '',
      throughput: durationMs > 0 ? (metric.completion_tokens ?? 0) / (durationMs / 1000) : 0,
    }
  }), [metrics])
  const costData = useMemo(() => metrics.map(metric => ({ date: metric.date || '', cost: metric.total_cost ?? 0 })), [metrics])
  const userRows = useMemo(() => {
    const entries = usersUsage.data || []
    const buildRow = (
      id: string,
      title: string,
      detail: string | undefined,
      userTotals: ReturnType<typeof aggregateMetrics>,
      source: TypesUsersAggregatedUsageMetric | null,
    ) => ({
      id,
      _data: source,
      _tokens: userTotals.tokens,
      name: (
        <Box>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>{title}</Typography>
          {detail && <Typography variant="caption" color="text.secondary">{detail}</Typography>}
        </Box>
      ),
      requests: <Typography variant="body2">{userTotals.requests.toLocaleString()}</Typography>,
      tokens: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatCompact(userTotals.tokens)}</Typography>,
      share: <Typography variant="body2" color="text.secondary">{formatPercent(totals.tokens > 0 ? userTotals.tokens / totals.tokens : 0)}</Typography>,
      cache: <Typography variant="body2">{formatPercent(getCacheHitRatio({
        total_tokens: userTotals.tokens,
        completion_tokens: userTotals.output,
        cache_read_tokens: userTotals.cacheRead,
      }))}</Typography>,
      latency: <Typography variant="body2">{formatMs(userTotals.latencyMs)}</Typography>,
      throughput: <Typography variant="body2">{formatRate(userTotals.throughput)}</Typography>,
      cost: <Typography variant="body2">{formatCost(userTotals.cost)}</Typography>,
    })

    const rows = entries.map((entry: TypesUsersAggregatedUsageMetric) => {
      const user = entry.user
      return buildRow(
        user?.id || user?.email || 'unknown',
        user?.full_name || user?.username || user?.email || 'Unknown user',
        user?.email && user.email !== user.full_name ? user.email : undefined,
        aggregateMetrics(entry.metrics || []),
        entry,
      )
    })

    const attributed = aggregateMetrics(entries.flatMap(entry => entry.metrics || []))
    const unattributedRequests = Math.max(totals.requests - attributed.requests, 0)
    const unattributedTokens = Math.max(totals.tokens - attributed.tokens, 0)
    if (unattributedRequests > 0 || unattributedTokens > 0) {
      const durationMs = Math.max(totals.durationMs - attributed.durationMs, 0)
      const output = Math.max(totals.output - attributed.output, 0)
      rows.push(buildRow(
        'unattributed',
        'Unattributed',
        'Deleted, system, or missing user',
        {
          input: Math.max(totals.input - attributed.input, 0),
          output,
          cacheRead: Math.max(totals.cacheRead - attributed.cacheRead, 0),
          cacheWrite: Math.max(totals.cacheWrite - attributed.cacheWrite, 0),
          tokens: unattributedTokens,
          requests: unattributedRequests,
          cost: Math.max(totals.cost - attributed.cost, 0),
          durationMs,
          requestBytes: Math.max(totals.requestBytes - attributed.requestBytes, 0),
          responseBytes: Math.max(totals.responseBytes - attributed.responseBytes, 0),
          latencyMs: unattributedRequests > 0 ? durationMs / unattributedRequests : 0,
          throughput: durationMs > 0 ? output / (durationMs / 1000) : 0,
        },
        null,
      ))
    }

    return rows.sort((a, b) => b._tokens - a._tokens)
  }, [usersUsage.data, totals])

  const handleBack = () => {
    if (onBack) {
      onBack()
      return
    }
    if (window.history.length > 1) {
      window.history.back()
      return
    }
    router.navigate('org_providers', { org_id: orgId })
  }

  const handleRangeChange = (_: React.MouseEvent<HTMLElement>, next: RangeKey | null) => {
    if (!next) return
    const days = next === '7d' ? 7 : next === '30d' ? 30 : 90
    setRange(next)
    setFrom(rangeFrom(days))
    setTo(today)
  }

  const page = (children: React.ReactNode, title = 'Provider') => (
    embedded ? (
      <Box sx={{ pb: 4 }}>{children}</Box>
    ) : (
      <Page breadcrumbTitle={title} breadcrumbShowHome orgBreadcrumbs>
        <Container maxWidth="xl"><Box sx={{ mt: 3, pb: 4 }}>{children}</Box></Container>
      </Page>
    )
  )

  if (providers.isLoading) {
    return page(<Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}><CircularProgress size={24} /></Box>)
  }

  if (!provider) {
    return page(
      <Stack spacing={2} alignItems="flex-start">
        <Button startIcon={<ArrowLeft size={17} />} onClick={handleBack}>Back to providers</Button>
        <Alert severity="warning">Provider not found or you do not have access to it.</Alert>
      </Stack>,
    )
  }

  const isLocal = provider.name?.includes('lmstudio') || provider.name?.includes('ollama') || provider.base_url?.includes(':1234') || provider.base_url?.includes(':11434')
  const hasError = provider.status === 'error'
  const modelCount = provider.available_models?.length || provider.models?.length || 0

  return page(
    <Stack spacing={2.5}>
      <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', md: 'center' }} spacing={2}>
        <Stack direction="row" spacing={1.5} alignItems="flex-start">
          <Button aria-label="Back to providers" onClick={handleBack} sx={{ minWidth: 34, width: 34, height: 34, p: 0, color: 'text.secondary' }}>
            <ArrowLeft size={18} />
          </Button>
          <Box sx={{ width: 46, height: 46, borderRadius: 2, bgcolor: 'action.hover', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
            <ProviderEndpointIcon endpoint={provider} size={27} />
          </Box>
          <Box>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
              <Typography variant="h5">{provider.name}</Typography>
              <Stack direction="row" spacing={0.5} alignItems="center" sx={{ color: hasError ? 'error.main' : 'success.main' }}>
                {hasError ? <CircleAlert size={15} /> : <CheckCircle2 size={15} />}
                <Typography variant="caption" sx={{ fontWeight: 600 }}>{hasError ? 'Connection error' : 'Connected'}</Typography>
              </Stack>
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, maxWidth: 760 }}>
              {provider.description || 'OpenAI-compatible inference provider'}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5, fontFamily: 'monospace', overflowWrap: 'anywhere' }}>
              {provider.base_url || 'Endpoint managed by server configuration'}
            </Typography>
          </Box>
        </Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
          {embedded && account.admin && provider.id && provider.id !== '-' && (
            <Button variant="outlined" startIcon={<Pencil size={17} />} onClick={() => setEditOpen(true)}>
              Edit provider
            </Button>
          )}
          <ToggleButtonGroup value={range} exclusive size="small" onChange={handleRangeChange}>
            <ToggleButton value="7d">7D</ToggleButton>
            <ToggleButton value="30d">30D</ToggleButton>
            <ToggleButton value="90d">90D</ToggleButton>
          </ToggleButtonGroup>
          <TextField size="small" type="date" label="From" value={from} onChange={event => { setRange(null); setFrom(event.target.value) }} InputLabelProps={{ shrink: true }} />
          <TextField size="small" type="date" label="To" value={to} onChange={event => { setRange(null); setTo(event.target.value) }} InputLabelProps={{ shrink: true }} />
        </Stack>
      </Stack>

      {hasError && provider.error && <Alert severity="error">{provider.error}</Alert>}
      {usage.error && <Alert severity="error">Failed to load provider telemetry: {(usage.error as Error).message}</Alert>}

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(3, minmax(0, 1fr))', xl: 'repeat(6, minmax(0, 1fr))' }, gap: '1px', bgcolor: 'divider', borderTop: '1px solid', borderBottom: '1px solid', borderColor: 'divider' }}>
        <OverviewStat flat={embedded} label="Processed tokens" value={formatCompact(totals.tokens)} detail={`${formatCompact(activeDays ? totals.tokens / activeDays : 0)} per active day`} icon={<Database size={14} />} />
        <OverviewStat flat={embedded} label="LLM calls" value={totals.requests.toLocaleString()} detail={`${activeDays || 0} active day${activeDays === 1 ? '' : 's'}`} icon={<Activity size={14} />} />
        <OverviewStat flat={embedded} label="Amount charged" value={provider.billing_enabled ? formatCost(totals.cost) : '$0.00'} detail={provider.billing_enabled ? 'Billing enabled' : `${formatCost(totals.cost)} API-rate equivalent`} icon={<Database size={14} />} />
        <OverviewStat flat={embedded} label="Average latency" value={formatMs(totals.latencyMs)} detail="End-to-end per LLM call" icon={<Gauge size={14} />} />
        <OverviewStat flat={embedded} label="Output throughput" value={formatRate(totals.throughput)} detail="Output tokens / request duration" icon={<Activity size={14} />} />
        <OverviewStat flat={embedded} label="Cache hit ratio" value={formatPercent(cacheHitRatio)} detail={`${formatCompact(totals.cacheRead)} cached input tokens`} icon={<Database size={14} />} />
      </Box>

      {usage.isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}><CircularProgress size={24} /></Box>
      ) : (
        <>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '2fr 1fr' }, gap: 2 }}>
            <ShadcnAreaChart title="DAILY TOKEN VOLUME" headline={formatCompact(totals.tokens)} data={chartData} series={TOKEN_SERIES} valueFormatter={formatCompact} />
            <ShadcnAreaChart title="DAILY API-RATE COST" headline={formatCost(totals.cost)} data={costData} series={COST_SERIES} valueFormatter={formatCost} hideLegend />
          </Box>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' }, gap: 2 }}>
            <ShadcnAreaChart title="END-TO-END LATENCY" headline={formatMs(totals.latencyMs)} data={latencyData} series={LATENCY_SERIES} valueFormatter={formatMs} variant="line" hideLegend zeroIsData yAxisWidth={92} />
            <ShadcnAreaChart title="OUTPUT THROUGHPUT" headline={formatRate(totals.throughput)} data={throughputData} series={THROUGHPUT_SERIES} valueFormatter={formatRate} variant="line" hideLegend zeroIsData yAxisWidth={92} />
          </Box>
        </>
      )}

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(5, minmax(0, 1fr))' }, gap: '1px', bgcolor: 'divider', borderTop: '1px solid', borderBottom: '1px solid', borderColor: 'divider' }}>
        <OverviewStat flat={embedded} label="Uncached input" value={formatCompact(Math.max(totals.input - totals.cacheRead - totals.cacheWrite, 0))} detail={`${formatCompact(totals.input)} total input`} icon={<Database size={14} />} />
        <OverviewStat flat={embedded} label="Output tokens" value={formatCompact(totals.output)} detail={`${formatPercent(totals.tokens > 0 ? totals.output / totals.tokens : null)} of processed tokens`} icon={<Activity size={14} />} />
        <OverviewStat flat={embedded} label="Cache writes" value={formatCompact(totals.cacheWrite)} detail="Input added to provider cache" icon={<Database size={14} />} />
        <OverviewStat flat={embedded} label="Request traffic" value={formatBytes(totals.requestBytes)} detail="Payload sent to provider" icon={<Server size={14} />} />
        <OverviewStat flat={embedded} label="Response traffic" value={formatBytes(totals.responseBytes)} detail="Payload returned by provider" icon={<Server size={14} />} />
      </Box>

      {account.admin && (
        <Box>
          <Stack direction="row" justifyContent="space-between" alignItems="baseline" sx={{ mb: 1.25 }}>
            <Typography variant="h6" sx={{ fontSize: '1rem' }}>Top consumers</Typography>
            <Typography variant="caption" color="text.secondary">Across all usage visible to this global endpoint</Typography>
          </Stack>
          <SimpleTable authenticated fields={userFields} data={userRows} compact loading={usersUsage.isLoading} />
        </Box>
      )}

      <Box>
        <Typography variant="h6" sx={{ fontSize: '1rem', mb: 1.25 }}>Endpoint details</Typography>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr 1fr', md: 'repeat(5, minmax(0, 1fr))' }, gap: '1px', bgcolor: 'divider', borderTop: '1px solid', borderBottom: '1px solid', borderColor: 'divider' }}>
          <OverviewStat label="Endpoint type" value={provider.endpoint_type || '—'} detail="Visibility scope" icon={<Server size={14} />} />
          <OverviewStat label="Owner" value={provider.owner === 'system' ? 'System' : provider.owner || '—'} detail={provider.owner_type || 'Unknown owner type'} icon={<Server size={14} />} />
          <OverviewStat label="Models discovered" value={modelCount.toLocaleString()} detail={modelCount ? 'Available from this endpoint' : 'No model list reported'} icon={<Database size={14} />} />
          <OverviewStat label="Billing" value={provider.billing_enabled ? 'Enabled' : 'Disabled'} detail={provider.billing_enabled ? 'Usage is chargeable' : 'No Helix charge'} icon={<Activity size={14} />} />
          <OverviewStat label="Status" value={hasError ? 'Error' : 'Connected'} detail={provider.status || 'Provider is configured'} icon={hasError ? <CircleAlert size={14} /> : <CheckCircle2 size={14} />} />
        </Box>
      </Box>

      {isLocal && provider.id && (
        <Box>
          <Typography variant="h6" sx={{ fontSize: '1rem', mb: 1.25 }}>Local models</Typography>
          <LMStudioModels endpointId={provider.id} />
        </Box>
      )}

      <EditProviderEndpointDialog
        open={editOpen}
        endpoint={provider}
        onClose={() => setEditOpen(false)}
        refreshData={() => { void providers.refetch() }}
      />
    </Stack>,
    provider.name || 'Provider',
  )
}
