import React, { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import Typography from '@mui/material/Typography'

// Building blocks shared by the org usage pages (LLM usage, web search), so
// they read as one system: header with range controls, headline number,
// proportional breakdown rows, stat strip and titled sections.

export type UsageRangeKey = '7d' | '30d' | '90d'

export const USAGE_RANGE_DAYS: Record<UsageRangeKey, number> = { '7d': 7, '30d': 30, '90d': 90 }

export const UsagePageHeader: FC<{
  title: string
  description: string
  actions: ReactNode
}> = ({ title, description, actions }) => (
  <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', md: 'center' }} spacing={2}>
    <Box>
      <Typography variant="h5" component="h2" sx={{ mb: 1 }}>{title}</Typography>
      <Typography variant="body2" color="text.secondary">{description}</Typography>
    </Box>
    {actions}
  </Stack>
)

export const UsageRangeControls: FC<{
  range: UsageRangeKey | null
  from: string
  to: string
  onRangeChange: (range: UsageRangeKey) => void
  onFromChange: (from: string) => void
  onToChange: (to: string) => void
}> = ({ range, from, to, onRangeChange, onFromChange, onToChange }) => (
  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
    <ToggleButtonGroup
      value={range}
      exclusive
      size="small"
      onChange={(_: React.MouseEvent<HTMLElement>, next: UsageRangeKey | null) => next && onRangeChange(next)}
    >
      <ToggleButton value="7d">7D</ToggleButton>
      <ToggleButton value="30d">30D</ToggleButton>
      <ToggleButton value="90d">90D</ToggleButton>
    </ToggleButtonGroup>
    <TextField size="small" label="From" type="date" value={from} onChange={e => onFromChange(e.target.value)} InputLabelProps={{ shrink: true }} />
    <TextField size="small" label="To" type="date" value={to} onChange={e => onToChange(e.target.value)} InputLabelProps={{ shrink: true }} />
  </Stack>
)

export const UsageHeadline: FC<{ label: string; value: string; detail: string }> = ({ label, value, detail }) => (
  <Box>
    <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase', letterSpacing: '0.08em' }}>
      {label}
    </Typography>
    <Typography sx={{ mt: 0.25, fontSize: { xs: '2.25rem', md: '2.6rem' }, lineHeight: 1.1, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
      {value}
    </Typography>
    <Typography variant="caption" color="text.secondary">{detail}</Typography>
  </Box>
)

// UsageShareRow is one item of a breakdown list: label, value, and a bar
// showing the item's share of the whole.
export const UsageShareRow: FC<{
  icon: ReactNode
  label: string
  value: string
  share: number
  color: string
  caption: string
}> = ({ icon, label, value, share, color, caption }) => (
  <Box>
    <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
        {icon}
        <Typography variant="body2" noWrap>{label}</Typography>
      </Stack>
      <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums' }}>{value}</Typography>
    </Stack>
    <Box sx={{ height: 4, bgcolor: 'action.hover', borderRadius: 4, overflow: 'hidden', mt: 0.75 }}>
      <Box sx={{ width: `${Math.min(share * 100, 100)}%`, height: '100%', bgcolor: color }} />
    </Box>
    <Typography variant="caption" color="text.secondary">{caption}</Typography>
  </Box>
)

export const OverviewStat: FC<{ label: string; value: string; detail: string }> = ({ label, value, detail }) => (
  <Box sx={{ bgcolor: 'background.default', px: 2, py: 1.5, minWidth: 0 }}>
    <Typography variant="caption" color="text.secondary">{label}</Typography>
    <Typography sx={{ fontSize: '1.1rem', lineHeight: 1.45, fontVariantNumeric: 'tabular-nums' }}>{value}</Typography>
    <Typography variant="caption" color="text.secondary" noWrap>{detail}</Typography>
  </Box>
)

// OverviewStatStrip lays OverviewStats out as a hairline-divided row.
export const OverviewStatStrip: FC<{ columns: number; children: ReactNode }> = ({ columns, children }) => (
  <Box
    sx={{
      display: 'grid',
      gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: `repeat(${columns}, minmax(0, 1fr))` },
      gap: '1px',
      bgcolor: 'divider',
      borderTop: '1px solid',
      borderBottom: '1px solid',
      borderColor: 'divider',
    }}
  >
    {children}
  </Box>
)

export const UsageSection: FC<{ title: string; action?: ReactNode; children: ReactNode }> = ({ title, action, children }) => (
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
