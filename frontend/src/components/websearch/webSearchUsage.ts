import { useTheme } from '@mui/material/styles'

import useThemeConfig from '../../hooks/useThemeConfig'
import { TypesWebSearchEndpoint, TypesWebSearchUsageDay } from '../../api/api'

export type UsageMetric = 'spend' | 'requests'

// Chart stacking order, bottom to top.
export const ENDPOINTS: TypesWebSearchEndpoint[] = [
  TypesWebSearchEndpoint.WebSearchEndpointSearch,
  TypesWebSearchEndpoint.WebSearchEndpointContents,
  TypesWebSearchEndpoint.WebSearchEndpointAnswer,
  TypesWebSearchEndpoint.WebSearchEndpointFindSimilar,
]

export const ENDPOINT_LABELS: Record<TypesWebSearchEndpoint, string> = {
  [TypesWebSearchEndpoint.WebSearchEndpointSearch]: 'Search',
  [TypesWebSearchEndpoint.WebSearchEndpointContents]: 'Contents',
  [TypesWebSearchEndpoint.WebSearchEndpointAnswer]: 'Answer',
  [TypesWebSearchEndpoint.WebSearchEndpointFindSimilar]: 'Find similar',
}

// Series colours: brand cyan and the neutral primary for the two common
// endpoints, amber and magenta for the rest, so no two read as the same hue.
export const useEndpointColors = (): Record<TypesWebSearchEndpoint, string> => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const isLight = theme.palette.mode === 'light'
  return {
    [TypesWebSearchEndpoint.WebSearchEndpointSearch]: theme.palette.secondary.main,
    [TypesWebSearchEndpoint.WebSearchEndpointContents]: theme.palette.primary.main,
    [TypesWebSearchEndpoint.WebSearchEndpointAnswer]: theme.palette.warning.main,
    [TypesWebSearchEndpoint.WebSearchEndpointFindSimilar]: isLight ? themeConfig.magentaDark : themeConfig.magentaRoot,
  }
}

// The usage API buckets by UTC day, so ranges are computed in UTC too.
export const utcDay = (date: Date): string => date.toISOString().slice(0, 10)

export const rangeBounds = (days: number, now = new Date()): { from: string; to: string } => {
  const to = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()))
  const from = new Date(to)
  from.setUTCDate(from.getUTCDate() - (days - 1))
  return { from: utcDay(from), to: utcDay(to) }
}

export type DailyRow = { date: string; [endpoint: string]: number | string }

// The API lists only day+endpoint pairs that had calls; the chart needs every
// day in the range, zero-filled.
export const buildDailyRows = (
  days: TypesWebSearchUsageDay[],
  from: string,
  to: string,
  metric: UsageMetric,
): DailyRow[] => {
  const rows = new Map<string, DailyRow>()
  const end = new Date(`${to}T00:00:00Z`)
  for (let d = new Date(`${from}T00:00:00Z`); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    const date = utcDay(d)
    rows.set(date, { date, search: 0, contents: 0, answer: 0, findSimilar: 0 })
  }
  for (const day of days) {
    const row = day.date ? rows.get(day.date) : undefined
    if (!row || !day.endpoint) continue
    row[day.endpoint] = Number(row[day.endpoint]) + (metric === 'spend' ? (day.costDollars ?? 0) : (day.requests ?? 0))
  }
  return Array.from(rows.values())
}

export const formatDollars = (value: number | undefined, digits: number): string =>
  `$${(value ?? 0).toFixed(digits)}`

export const formatDuration = (ms: number | undefined): string => `${((ms ?? 0) / 1000).toFixed(2)}s`
