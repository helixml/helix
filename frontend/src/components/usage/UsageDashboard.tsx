import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import CircularProgress from '@mui/material/CircularProgress'
import Pagination from '@mui/material/Pagination'

import SimpleTable, { ITableField } from '../widgets/SimpleTable'
import { useUsageSummary, useUsageGrouped, UsageFilter, UsageGrouping } from '../../services/usageAggregateService'

// ---------------------------------------------------------------------------
// Formatting helpers. Pricing in the API is dollars-per-token; aggregate
// rows already carry dollars, so just format directly.
// ---------------------------------------------------------------------------
function formatCost(n: number | undefined): string {
  if (!n) return '$0'
  if (n >= 1000) return `$${(n / 1000).toFixed(2)}k`
  if (n >= 1) return `$${n.toFixed(2)}`
  return `$${n.toFixed(4)}`
}

function formatTokens(n: number | undefined): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return n.toString()
}

function formatDate(s: string | undefined): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

// ---------------------------------------------------------------------------
// Date-range presets. Custom range support comes later; for now the
// preset list covers the dashboard use case.
// ---------------------------------------------------------------------------
type PresetKey = '1d' | '7d' | '30d' | '90d'

const presetWindows: Record<PresetKey, { label: string, days: number }> = {
  '1d': { label: 'Last 24 hours', days: 1 },
  '7d': { label: 'Last 7 days', days: 7 },
  '30d': { label: 'Last 30 days', days: 30 },
  '90d': { label: 'Last 90 days', days: 90 },
}

function presetToFromTo(p: PresetKey): { from: string, to: string } {
  const to = new Date()
  const from = new Date(to.getTime() - presetWindows[p].days * 24 * 60 * 60 * 1000)
  return { from: from.toISOString(), to: to.toISOString() }
}

// ---------------------------------------------------------------------------
// Stat tile.
// ---------------------------------------------------------------------------
const StatTile: FC<{ label: string, value: string, hint?: string }> = ({ label, value, hint }) => (
  <Paper
    variant="outlined"
    sx={{
      p: 1.5,
      flex: '1 1 0',
      minWidth: 140,
      background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
      border: '1px solid rgba(255,255,255,0.06)',
      borderRadius: 2,
    }}
  >
    <Typography variant="caption" sx={{ fontSize: '0.65rem', textTransform: 'uppercase', color: 'text.secondary' }}>
      {label}
    </Typography>
    <Typography variant="body2" sx={{ fontSize: '1.1rem', fontFamily: 'monospace', fontWeight: 600, mt: 0.5 }}>
      {value}
    </Typography>
    {hint && (
      <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'text.secondary' }}>
        {hint}
      </Typography>
    )}
  </Paper>
)

// ---------------------------------------------------------------------------
// Column sets per grouping. These mirror the row shapes in
// api/pkg/types/types.go - UsageByUser, UsageBySession etc. The
// generated TS client types `rows` as `unknown`, so we cast in the
// table-data builder.
// ---------------------------------------------------------------------------
type Row = Record<string, any>

const groupColumns: Record<UsageGrouping, ITableField[]> = {
  org: [
    { name: 'organization_name', title: 'Organization' },
    { name: 'request_count', title: 'Requests', numeric: true },
    { name: 'total_tokens', title: 'Tokens', numeric: true },
    { name: 'total_cost', title: 'Cost', numeric: true },
    { name: 'user_count', title: 'Users', numeric: true },
    { name: 'session_count', title: 'Sessions', numeric: true },
    { name: 'top_model', title: 'Top model' },
    { name: 'last_activity', title: 'Last activity' },
  ],
  user: [
    { name: 'email', title: 'User' },
    { name: 'request_count', title: 'Requests', numeric: true },
    { name: 'total_tokens', title: 'Tokens', numeric: true },
    { name: 'total_cost', title: 'Cost', numeric: true },
    { name: 'session_count', title: 'Sessions', numeric: true },
    { name: 'top_model', title: 'Top model' },
    { name: 'last_activity', title: 'Last activity' },
  ],
  project: [
    { name: 'name', title: 'Project / App' },
    { name: 'kind', title: 'Kind' },
    { name: 'request_count', title: 'Requests', numeric: true },
    { name: 'total_tokens', title: 'Tokens', numeric: true },
    { name: 'total_cost', title: 'Cost', numeric: true },
    { name: 'session_count', title: 'Sessions', numeric: true },
  ],
  session: [
    { name: 'session_id', title: 'Session' },
    { name: 'model', title: 'Model' },
    { name: 'request_count', title: 'Calls', numeric: true },
    { name: 'total_tokens', title: 'Tokens', numeric: true },
    { name: 'total_cost', title: 'Cost', numeric: true },
    { name: 'ended_at', title: 'Last call' },
  ],
  model: [
    { name: 'model', title: 'Model' },
    { name: 'provider', title: 'Provider' },
    { name: 'request_count', title: 'Requests', numeric: true },
    { name: 'total_tokens', title: 'Tokens', numeric: true },
    { name: 'total_cost', title: 'Cost', numeric: true },
    { name: 'unique_users', title: 'Users', numeric: true },
    { name: 'unique_sessions', title: 'Sessions', numeric: true },
  ],
}

function rowToTableData(groupBy: UsageGrouping, raw: Row): Row {
  // Format the numeric and time columns for display, but keep the
  // raw values on `_data` so future drill-down handlers can recover.
  const base: Row = {
    id: raw.user_id || raw.session_id || raw.project_id || raw.organization_id || `${raw.provider}/${raw.model}`,
    _data: raw,
    total_cost: formatCost(raw.total_cost),
    total_tokens: formatTokens(raw.total_tokens),
    request_count: raw.request_count?.toLocaleString() ?? '0',
    user_count: raw.user_count?.toLocaleString() ?? '0',
    session_count: raw.session_count?.toLocaleString() ?? '0',
    unique_users: raw.unique_users?.toLocaleString() ?? '0',
    unique_sessions: raw.unique_sessions?.toLocaleString() ?? '0',
    last_activity: formatDate(raw.last_activity),
    ended_at: formatDate(raw.ended_at),
  }
  switch (groupBy) {
    case 'org':
      base.organization_name = raw.organization_name || raw.organization_id || '(unnamed)'
      base.top_model = raw.top_model || ''
      break
    case 'user':
      base.email = raw.email || raw.user_id
      base.top_model = raw.top_model || ''
      break
    case 'project':
      base.name = raw.name || raw.project_id || raw.app_id || '(unnamed)'
      base.kind = raw.kind || ''
      break
    case 'session':
      base.session_id = raw.name || raw.session_id
      base.model = raw.model || ''
      break
    case 'model':
      base.model = raw.model
      base.provider = raw.provider
      break
  }
  return base
}

// ---------------------------------------------------------------------------
// Main component.
// ---------------------------------------------------------------------------
export interface UsageDashboardProps {
  // Locked filters. If `org_id` is set the user cannot edit it (used
  // by the org-owner surface, future PR). For now AdminUsage passes
  // nothing locked and all groupings are allowed.
  initialFilter?: UsageFilter
  lockedFilterKeys?: (keyof UsageFilter)[]
  allowedGroupings?: UsageGrouping[]
}

const defaultGroupings: UsageGrouping[] = ['org', 'user', 'project', 'session', 'model']

const UsageDashboard: FC<UsageDashboardProps> = ({
  initialFilter = {},
  lockedFilterKeys = [],
  allowedGroupings = defaultGroupings,
}) => {
  const [preset, setPreset] = useState<PresetKey>('7d')
  const window = useMemo(() => presetToFromTo(preset), [preset])
  const [groupBy, setGroupBy] = useState<UsageGrouping>(allowedGroupings[0])
  const [page, setPage] = useState(1)
  const pageSize = 25

  // The effective filter combines the date window with whatever the
  // parent locked in (e.g. org_id).
  const filter: UsageFilter = {
    ...initialFilter,
    from: window.from,
    to: window.to,
    page,
    page_size: pageSize,
  }

  const { data: summary, isLoading: summaryLoading } = useUsageSummary(filter)
  const { data: grouped, isLoading: groupedLoading } = useUsageGrouped(groupBy, filter)

  const tableData = useMemo(() => {
    const rows = (grouped?.rows ?? []) as Row[]
    return rows.map(r => rowToTableData(groupBy, r))
  }, [grouped, groupBy])

  const handlePreset = (_: unknown, next: PresetKey | null) => {
    if (next) {
      setPreset(next)
      setPage(1)
    }
  }

  return (
    <Stack spacing={3}>
      {/* Date range */}
      <Stack direction="row" spacing={2} alignItems="center" justifyContent="flex-end">
        <ToggleButtonGroup value={preset} exclusive onChange={handlePreset} size="small">
          {(Object.keys(presetWindows) as PresetKey[]).map(k => (
            <ToggleButton key={k} value={k}>{presetWindows[k].label}</ToggleButton>
          ))}
        </ToggleButtonGroup>
      </Stack>

      {/* Locked-filter hint */}
      {lockedFilterKeys.length > 0 && initialFilter.org_id && (
        <Typography variant="caption" color="text.secondary">
          Scope: organization <strong>{initialFilter.org_id}</strong>
        </Typography>
      )}

      {/* Overview tiles */}
      <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
        <StatTile label="Total cost" value={formatCost(summary?.total_cost)} hint={summaryLoading ? 'loading...' : undefined} />
        <StatTile label="Total tokens" value={formatTokens(summary?.total_tokens)} />
        <StatTile label="Input" value={formatTokens(summary?.prompt_tokens)} />
        <StatTile label="Output" value={formatTokens(summary?.completion_tokens)} />
        <StatTile label="Cache read" value={formatTokens(summary?.cache_read_tokens)} />
        <StatTile label="Cache write" value={formatTokens(summary?.cache_write_tokens)} />
        <StatTile label="Requests" value={summary?.request_count?.toLocaleString() ?? '0'} />
        <StatTile label="Active users" value={summary?.active_users?.toLocaleString() ?? '0'} />
      </Box>

      {/* Group-by tabs */}
      <Tabs
        value={groupBy}
        onChange={(_, v) => { setGroupBy(v as UsageGrouping); setPage(1) }}
        variant="scrollable"
        scrollButtons="auto"
      >
        {allowedGroupings.map(g => (
          <Tab key={g} value={g} label={
            ({ org: 'Organizations', user: 'Users', project: 'Projects', session: 'Sessions', model: 'Models' })[g]
          } />
        ))}
      </Tabs>

      {/* Table */}
      {groupedLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          <SimpleTable
            authenticated
            fields={groupColumns[groupBy]}
            data={tableData}
            hideHeaderIfEmpty
          />
          {grouped && grouped.total_pages > 1 && (
            <Stack direction="row" justifyContent="flex-end" sx={{ mt: 1 }}>
              <Pagination
                count={grouped.total_pages}
                page={grouped.page || 1}
                onChange={(_, p) => setPage(p)}
                size="small"
              />
            </Stack>
          )}
        </>
      )}
    </Stack>
  )
}

export default UsageDashboard
