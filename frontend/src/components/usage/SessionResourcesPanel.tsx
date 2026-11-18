// SessionResourcesPanel shows how a session's sandbox uses its CPU, memory,
// I/O and disk: what it needs (p95 CPU, peak memory) against its limits, and
// whether it is starved (throttling, pressure stalls, OOM kills). Sized for
// the session workspace's Usage tab, below the LLM usage.

import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'

import { SessionResources, useSessionResources } from '../../services/sessionResourcesService'
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart'
import { CompactTable, OverviewStat, OverviewStatStrip, UsageChartGrid, UsageGroup } from './UsageOverview'

const WINDOWS = [
  { hours: 1, label: '1h' },
  { hours: 24, label: '24h' },
  { hours: 168, label: '7d' },
  { hours: 720, label: '30d' },
]

// Limits stay out of the charts: an idle sandbox under a 12-core limit would
// draw as a flat line on the axis. The tiles and footer carry them.
const CPU_SERIES: ShadcnSeries[] = [{ key: 'used', label: 'Used', color: '#3b82f6' }]
const MEMORY_SERIES: ShadcnSeries[] = [{ key: 'used', label: 'Used', color: '#a855f7' }]
const STALL_SERIES: ShadcnSeries[] = [
  { key: 'cpu', label: 'CPU', color: '#3b82f6' },
  { key: 'memory', label: 'Memory', color: '#a855f7' },
  { key: 'io', label: 'I/O', color: '#f59e0b' },
  { key: 'throttled', label: 'CPU throttled', color: '#ef4444' },
]
// Disk read is teal so it never reuses cache-read green from the LLM charts.
const IO_SERIES: ShadcnSeries[] = [
  { key: 'read', label: 'Read', color: '#0891b2' },
  { key: 'write', label: 'Write', color: '#f59e0b' },
]

// headroom tops a chart's axis a quarter above its peak, so a steady series
// sits inside the plot instead of along its upper edge.
const headroom = (values: number[], floor: number) => Math.max(floor, ...values) * 1.25

export const formatBytes = (n: number): string => {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)} ${units[i]}`
}

const formatCores = (n: number): string => (n < 10 ? n.toFixed(2) : n.toFixed(1))
const formatPct = (n: number): string => `${n < 10 && n > 0 ? n.toFixed(1) : Math.round(n)}%`
const limitNote = (limit: number, format: (n: number) => string) => (limit > 0 ? `limit ${format(limit)}` : 'no limit')

export const buildResourceRows = (r: SessionResources) =>
  r.points.map(p => ({
    date: new Date(p.timestamp).toISOString(),
    cpu: { used: p.cpu_cores },
    memory: { used: p.memory_bytes },
    stalls: { cpu: p.cpu_pressure_pct, memory: p.memory_pressure_pct, io: p.io_pressure_pct, throttled: p.cpu_throttled_pct },
    io: { read: p.io_read_bps, write: p.io_write_bps },
  }))

const rowsFor = <K extends 'cpu' | 'memory' | 'stalls' | 'io'>(rows: ReturnType<typeof buildResourceRows>, key: K) =>
  rows.map(row => ({ date: row.date, ...row[key] }))

export const SessionResourcesView: FC<{ resources: SessionResources }> = ({ resources }) => {
  const s = resources.summary
  const rows = useMemo(() => buildResourceRows(resources), [resources])
  const dedicated = resources.filesystems.filter(f => f.dedicated)
  const starved = s.oom_kills > 0 ? `${s.oom_kills} OOM kill${s.oom_kills === 1 ? '' : 's'}` : s.memory_high_events > 0 ? `${s.memory_high_events} memory.high hits` : 'no OOM kills'

  if (rows.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        {s.samples === 0
          ? 'No sandbox samples in this window. The desktop reports every 30 seconds while it runs.'
          : `${s.samples} sample${s.samples === 1 ? '' : 's'} in this window, not yet enough to measure usage: rates need two consecutive samples from the same desktop run.`}
      </Typography>
    )
  }

  return (
    <Stack spacing={2}>
      <OverviewStatStrip columns={4} fit="container">
        <OverviewStat emphasis label="CPU p95" value={`${formatCores(s.cpu_cores_p95)} cores`} detail={`Max ${formatCores(s.cpu_cores_max)} · ${limitNote(s.cpu_limit_cores, formatCores)}`} />
        <OverviewStat emphasis label="Memory max" value={formatBytes(s.memory_max_bytes)} detail={limitNote(s.memory_limit_bytes, formatBytes)} />
        <OverviewStat emphasis label="Stalled on I/O" value={formatPct(s.io_pressure_pct)} detail={`CPU ${formatPct(s.cpu_pressure_pct)} · memory ${formatPct(s.memory_pressure_pct)}`} />
        <OverviewStat emphasis label="CPU throttled" value={formatPct(s.cpu_throttled_pct)} detail={starved} />
      </OverviewStatStrip>

      <UsageChartGrid fit="container">
        <ShadcnAreaChart
          title="CPU"
          headline={`p95 ${formatCores(s.cpu_cores_p95)} cores${s.cpu_limit_cores > 0 ? ` of ${formatCores(s.cpu_limit_cores)}` : ''}`}
          data={rowsFor(rows, 'cpu')}
          series={CPU_SERIES}
          hideLegend
          valueFormatter={formatCores}
          stacked={false}
          variant="line"
          zeroIsData
          yDomain={[0, headroom(rows.map(r => r.cpu.used), 0.1)]}
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={48}
          showTime
        />
        <ShadcnAreaChart
          title="Memory"
          headline={`Max ${formatBytes(s.memory_max_bytes)}${s.memory_limit_bytes > 0 ? ` of ${formatBytes(s.memory_limit_bytes)}` : ''}`}
          data={rowsFor(rows, 'memory')}
          series={MEMORY_SERIES}
          hideLegend
          valueFormatter={formatBytes}
          stacked={false}
          variant="line"
          zeroIsData
          yDomain={[0, headroom(rows.map(r => r.memory.used), 1024 ** 3)]}
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={64}
          showTime
        />
        <ShadcnAreaChart
          title="Time stalled"
          headline={`I/O ${formatPct(s.io_pressure_pct)} · CPU throttled ${formatPct(s.cpu_throttled_pct)}`}
          data={rowsFor(rows, 'stalls')}
          series={STALL_SERIES}
          valueFormatter={formatPct}
          stacked={false}
          variant="line"
          yDomain={[0, 100]}
          zeroIsData
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={48}
          showTime
        />
        <ShadcnAreaChart
          title="Disk I/O"
          headline={`${formatBytes(s.io_read_bytes)} read · ${formatBytes(s.io_write_bytes)} written`}
          data={rowsFor(rows, 'io')}
          series={IO_SERIES}
          valueFormatter={v => `${formatBytes(v)}/s`}
          stacked={false}
          variant="line"
          zeroIsData
          cardHeight="auto"
          chartHeight={140}
          yAxisWidth={72}
          showTime
        />
      </UsageChartGrid>

      {resources.filesystems.length > 0 && (
        <CompactTable
          title="Filesystems"
          columns={[
            { key: 'paths', label: 'Mounted at' },
            { key: 'scope', label: 'Scope' },
            { key: 'used', label: 'Used' },
            { key: 'inodes', label: 'Inodes' },
          ]}
          rows={resources.filesystems.map(f => ({
            id: f.paths,
            title: f.paths,
            cells: {
              paths: f.paths,
              scope: f.dedicated ? 'This session' : 'Shared host volume',
              used: `${formatBytes(f.used_bytes)} of ${formatBytes(f.size_bytes)} (${formatPct((100 * f.used_bytes) / f.size_bytes)})`,
              inodes: f.inodes > 0 ? formatPct((100 * f.inodes_used) / f.inodes) : '—',
            },
          }))}
          footer={dedicated.length === 0 ? (
            <Typography variant="caption" color="text.secondary">
              This sandbox host keeps session data on shared volumes, so per-session disk use is not measured.
            </Typography>
          ) : undefined}
        />
      )}
    </Stack>
  )
}

const describe = (r?: SessionResources | null) => {
  const s = r?.summary
  if (!s || s.samples === 0) return 'CPU, memory, I/O and disk of this session\'s sandbox, sampled every 30 seconds.'
  const cpu = s.cpu_limit_cores > 0 ? `${formatCores(s.cpu_limit_cores)} CPU` : 'unlimited CPU'
  const mem = s.memory_limit_bytes > 0 ? `${formatBytes(s.memory_limit_bytes)} memory` : 'unlimited memory'
  return `Sampled every 30 seconds · ${s.samples} samples · limits ${cpu}, ${mem} · peak memory since start ${formatBytes(s.memory_peak_bytes)}`
}

const SessionResourcesPanel: FC<{ sessionId: string }> = ({ sessionId }) => {
  const [hours, setHours] = useState(24)
  const { data, isLoading, error } = useSessionResources(sessionId, hours)
  return (
    <UsageGroup
      title="Sandbox resources"
      description={describe(data)}
      action={(
        <ToggleButtonGroup size="small" exclusive value={hours} onChange={(_, v) => v && setHours(v)} aria-label="Time window">
          {WINDOWS.map(w => (
            <ToggleButton key={w.hours} value={w.hours} sx={{ px: 1.25, textTransform: 'none' }}>
              {w.label}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      )}
    >
      {isLoading ? (
        <CircularProgress size={18} />
      ) : error || !data ? (
        <Typography variant="body2" color="text.secondary">Sandbox resources are not available for this session.</Typography>
      ) : (
        <SessionResourcesView resources={data} />
      )}
    </UsageGroup>
  )
}

export default SessionResourcesPanel
