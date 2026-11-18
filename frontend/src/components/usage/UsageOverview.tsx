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

export const OverviewStat: FC<{ label: string; value: string; detail: string; emphasis?: boolean }> = ({ label, value, detail, emphasis }) => (
  <Box sx={{ bgcolor: 'background.paper', px: 2, py: 1.5, minWidth: 0 }}>
    <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }} noWrap>{label}</Typography>
    <Typography
      sx={{
        fontSize: emphasis ? '1.5rem' : '1.125rem',
        fontWeight: emphasis ? 600 : 500,
        lineHeight: 1.35,
        fontVariantNumeric: 'tabular-nums',
      }}
      noWrap
    >
      {value}
    </Typography>
    <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }} noWrap title={detail}>{detail}</Typography>
  </Box>
)

// OverviewStatStrip lays OverviewStats out as a hairline-divided row.
// Layouts inside a side panel size by the panel, not the window: their
// root sets `containerType: 'inline-size'` and children use `fit="container"`.
export type UsageFit = 'viewport' | 'container'

// Columns step down evenly (6 → 3 → 2, 4 → 4 → 2) so a row never ends in
// an orphan cell.
export const OverviewStatStrip: FC<{ columns: number; fit?: UsageFit; children: ReactNode }> = ({ columns, fit = 'viewport', children }) => (
  <Box
    sx={{
      display: 'grid',
      gridTemplateColumns: fit === 'container' ? 'repeat(2, minmax(0, 1fr))' : {
        xs: 'repeat(2, minmax(0, 1fr))',
        md: `repeat(${columns % 3 === 0 ? 3 : columns}, minmax(0, 1fr))`,
        lg: `repeat(${columns}, minmax(0, 1fr))`,
      },
      ...(fit === 'container' && {
        [`@container (min-width: ${columns * 190}px)`]: { gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` },
      }),
      gap: '1px',
      bgcolor: 'divider',
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 2,
      overflow: 'hidden',
    }}
  >
    {children}
  </Box>
)

// UsageGroup is one titled section of a usage page (LLM spend, sandbox
// compute, breakdowns), so related numbers, charts and tables read together.
export const UsageGroup: FC<{ title: string; description: string; action?: ReactNode; children: ReactNode }> = ({
  title, description, action, children,
}) => (
  <Box component="section" sx={{ pt: 1 }}>
    <Stack
      direction={{ xs: 'column', sm: 'row' }}
      alignItems={{ xs: 'flex-start', sm: 'flex-end' }}
      justifyContent="space-between"
      spacing={1}
      sx={{ mb: 2, pb: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}
    >
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="h6" component="h3" sx={{ fontSize: '1.125rem', fontWeight: 600 }}>{title}</Typography>
        <Typography variant="body2" color="text.secondary">{description}</Typography>
      </Box>
      {action}
    </Stack>
    <Stack spacing={2}>{children}</Stack>
  </Box>
)

// UsageChartGrid lays charts out two-up on wide screens, one-up on narrow.
export const UsageChartGrid: FC<{ fit?: UsageFit; children: ReactNode }> = ({ fit = 'viewport', children }) => (
  <Box
    sx={{
      display: 'grid',
      gap: 2,
      gridTemplateColumns: fit === 'container' ? 'minmax(0, 1fr)' : { xs: 'minmax(0, 1fr)', lg: 'repeat(2, minmax(0, 1fr))' },
      ...(fit === 'container' && {
        '@container (min-width: 640px)': { gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' },
      }),
    }}
  >
    {children}
  </Box>
)

export interface CompactColumn {
  key: string
  label: string
  // The first column is the row's name; the rest are right-aligned values.
}

// CompactTable is a titled table card that turns each row into a name line
// plus a line of labelled values when its container is narrow, so no column
// is ever clipped on a phone.
export const CompactTable: FC<{
  title: string
  columns: CompactColumn[]
  rows: Array<{ id: string; title?: string; cells: Record<string, ReactNode> }>
  footer?: ReactNode
}> = ({ title, columns, rows, footer }) => {
  const [nameCol, ...valueCols] = columns
  const cellSx = { py: 0.75, px: 1, borderTop: '1px solid', borderColor: 'divider', fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' } as const
  return (
    <Box sx={{ bgcolor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, minWidth: 0 }}>
      <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>{title}</Typography>
      <Box
        component="table"
        sx={{
          width: '100%',
          borderCollapse: 'collapse',
          fontSize: '0.8125rem',
          display: 'none',
          '@container (min-width: 560px)': { display: 'table' },
        }}
      >
        <thead>
          <tr>
            {columns.map((c, i) => (
              <Box component="th" key={c.key} sx={{ py: 0.75, px: 1, fontWeight: 500, color: 'text.secondary', textAlign: i === 0 ? 'left' : 'right', whiteSpace: 'nowrap' }}>
                {c.label}
              </Box>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map(r => (
            <tr key={r.id}>
              {/* width 100% + maxWidth 0: the name takes the spare width and ellipsizes, values never clip. */}
              <Box component="td" title={r.title} sx={{ ...cellSx, width: '100%', maxWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {r.cells[nameCol.key]}
              </Box>
              {valueCols.map(c => (
                <Box component="td" key={c.key} sx={{ ...cellSx, textAlign: 'right' }}>{r.cells[c.key]}</Box>
              ))}
            </tr>
          ))}
        </tbody>
      </Box>
      <Box sx={{ '@container (min-width: 560px)': { display: 'none' } }}>
        {rows.map(r => (
          <Box key={r.id} sx={{ py: 1, borderTop: '1px solid', borderColor: 'divider', fontSize: '0.8125rem' }}>
            <Box title={r.title} sx={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.cells[nameCol.key]}</Box>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 2, rowGap: 0.25, mt: 0.25, color: 'text.secondary', fontVariantNumeric: 'tabular-nums' }}>
              {valueCols.map(c => (
                <span key={c.key}>{c.label} <Box component="span" sx={{ color: 'text.primary' }}>{r.cells[c.key]}</Box></span>
              ))}
            </Box>
          </Box>
        ))}
      </Box>
      {footer && <Box sx={{ mt: 1 }}>{footer}</Box>}
    </Box>
  )
}

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
