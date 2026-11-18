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

import useLightTheme from '../../hooks/useLightTheme'
import { SessionResources, useSessionResources } from '../../services/sessionResourcesService'
import ShadcnAreaChart, { ShadcnSeries } from './ShadcnAreaChart'
import { SECTION_LABEL_SX, Tile } from './UsageTile'

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
const IO_SERIES: ShadcnSeries[] = [
  { key: 'read', label: 'Read', color: '#22c55e' },
  { key: 'write', label: 'Write', color: '#f59e0b' },
]

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
  const lightTheme = useLightTheme()
  const s = resources.summary
  const rows = useMemo(() => buildResourceRows(resources), [resources])
  const dedicated = resources.filesystems.filter(f => f.dedicated)
  const starved = s.oom_kills > 0 ? `${s.oom_kills} OOM kill${s.oom_kills === 1 ? '' : 's'}` : s.memory_high_events > 0 ? `${s.memory_high_events} memory.high hits` : 'no OOM kills'

  if (rows.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No sandbox samples in this window. The desktop reports every 30 seconds while it runs.
      </Typography>
    )
  }

  return (
    <Stack spacing={1.5}>
      <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))', gap: 1 }}>
        <Tile label="CPU p95" value={`${formatCores(s.cpu_cores_p95)} cores`} note={`max ${formatCores(s.cpu_cores_max)} · ${limitNote(s.cpu_limit_cores, formatCores)}`} />
        <Tile label="Memory max" value={formatBytes(s.memory_max_bytes)} note={limitNote(s.memory_limit_bytes, formatBytes)} />
        <Tile label="Stalled on I/O" value={formatPct(s.io_pressure_pct)} note={`CPU ${formatPct(s.cpu_pressure_pct)} · memory ${formatPct(s.memory_pressure_pct)}`} />
        <Tile label="CPU throttled" value={formatPct(s.cpu_throttled_pct)} note={starved} />
      </Box>

      <ShadcnAreaChart
        title="CPU cores"
        headline={`p95 ${formatCores(s.cpu_cores_p95)}${s.cpu_limit_cores > 0 ? ` of ${formatCores(s.cpu_limit_cores)}` : ''}`}
        data={rowsFor(rows, 'cpu')}
        series={CPU_SERIES}
        hideLegend
        valueFormatter={formatCores}
        stacked={false}
        variant="line"
        zeroIsData
        cardHeight="auto"
        chartHeight={120}
        yAxisWidth={48}
        showTime
      />
      <ShadcnAreaChart
        title="Memory"
        headline={`${formatBytes(s.memory_max_bytes)}${s.memory_limit_bytes > 0 ? ` of ${formatBytes(s.memory_limit_bytes)}` : ''}`}
        data={rowsFor(rows, 'memory')}
        series={MEMORY_SERIES}
        hideLegend
        valueFormatter={formatBytes}
        stacked={false}
        variant="line"
        zeroIsData
        cardHeight="auto"
        chartHeight={120}
        yAxisWidth={64}
        showTime
      />
      <ShadcnAreaChart
        title="Time stalled"
        headline={`I/O ${formatPct(s.io_pressure_pct)}`}
        data={rowsFor(rows, 'stalls')}
        series={STALL_SERIES}
        valueFormatter={formatPct}
        stacked={false}
        variant="line"
        yDomain={[0, 100]}
        zeroIsData
        cardHeight="auto"
        chartHeight={120}
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
        chartHeight={110}
        yAxisWidth={72}
        showTime
      />

      {resources.filesystems.length > 0 && (
        <Box>
          <Typography variant="caption" sx={SECTION_LABEL_SX}>Filesystems</Typography>
          <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse', mt: 0.5, fontSize: '0.78rem', fontVariantNumeric: 'tabular-nums' }}>
            <Box component="thead">
              <Box component="tr" sx={{ color: 'text.secondary', textAlign: 'left' }}>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5 }}>Mounted at</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5 }}>Scope</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Used</Box>
                <Box component="th" sx={{ fontWeight: 500, py: 0.5, textAlign: 'right' }}>Inodes</Box>
              </Box>
            </Box>
            <Box component="tbody">
              {resources.filesystems.map(f => (
                <Box component="tr" key={f.paths} sx={{ borderTop: '1px solid', borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(255,255,255,0.06)' }}>
                  <Box component="td" sx={{ py: 0.5, pr: 1, maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={f.paths}>
                    {f.paths}
                  </Box>
                  <Box component="td" sx={{ py: 0.5, color: 'text.secondary' }}>{f.dedicated ? 'this session' : 'shared host volume'}</Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>
                    {formatBytes(f.used_bytes)} / {formatBytes(f.size_bytes)} ({formatPct((100 * f.used_bytes) / f.size_bytes)})
                  </Box>
                  <Box component="td" sx={{ py: 0.5, textAlign: 'right' }}>{f.inodes > 0 ? formatPct((100 * f.inodes_used) / f.inodes) : '–'}</Box>
                </Box>
              ))}
            </Box>
          </Box>
          {dedicated.length === 0 && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
              This sandbox host keeps session data on shared volumes, so per-session disk use is not measured.
            </Typography>
          )}
        </Box>
      )}

      <Typography variant="caption" color="text.secondary">
        {s.samples} samples · {s.cpu_limit_cores > 0 ? `${formatCores(s.cpu_limit_cores)} CPU` : 'unlimited CPU'} ·{' '}
        {s.memory_limit_bytes > 0 ? `${formatBytes(s.memory_limit_bytes)} memory` : 'unlimited memory'} · peak since start {formatBytes(s.memory_peak_bytes)}
      </Typography>
    </Stack>
  )
}

const SessionResourcesPanel: FC<{ sessionId: string }> = ({ sessionId }) => {
  const [hours, setHours] = useState(24)
  const { data, isLoading, error } = useSessionResources(sessionId, hours)
  return (
    <Stack spacing={1}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
        <Typography variant="caption" sx={SECTION_LABEL_SX}>Sandbox resources</Typography>
        <ToggleButtonGroup size="small" exclusive value={hours} onChange={(_, v) => v && setHours(v)} aria-label="Time window">
          {WINDOWS.map(w => (
            <ToggleButton key={w.hours} value={w.hours} sx={{ py: 0.25, px: 1, fontSize: '0.72rem', textTransform: 'none' }}>
              {w.label}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      </Box>
      {isLoading ? (
        <CircularProgress size={18} />
      ) : error || !data ? (
        <Typography variant="body2" color="text.secondary">Sandbox resources are not available for this session.</Typography>
      ) : (
        <SessionResourcesView resources={data} />
      )}
    </Stack>
  )
}

export default SessionResourcesPanel
