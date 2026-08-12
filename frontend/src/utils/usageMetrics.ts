export interface CacheUsageMetric {
  total_tokens?: number
  completion_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
}

export interface CacheUsageTimeSeries {
  id?: string
  runtime?: string
  metrics?: Array<CacheUsageMetric & { date?: string }>
}

export type CacheHitRatioChartRow = { date: string; [key: string]: number | string }

export const getTotalInputTokens = (metric: CacheUsageMetric): number => (
  Math.max((metric.total_tokens ?? 0) - (metric.completion_tokens ?? 0), 0)
)

export const getUncachedInputTokens = (metric: CacheUsageMetric): number => (
  Math.max(
    getTotalInputTokens(metric) - (metric.cache_read_tokens ?? 0) - (metric.cache_write_tokens ?? 0),
    0,
  )
)

export const getCacheHitRatio = (metric: CacheUsageMetric): number | null => {
  const inputTokens = getTotalInputTokens(metric)
  if (inputTokens === 0) return null
  return (metric.cache_read_tokens ?? 0) / inputTokens
}

export const getAggregateCacheHitRatio = (metrics: CacheUsageMetric[]): number | null => {
  const totals = metrics.reduce((aggregate, metric) => ({
    total_tokens: aggregate.total_tokens + (metric.total_tokens ?? 0),
    completion_tokens: aggregate.completion_tokens + (metric.completion_tokens ?? 0),
    cache_read_tokens: aggregate.cache_read_tokens + (metric.cache_read_tokens ?? 0),
  }), { total_tokens: 0, completion_tokens: 0, cache_read_tokens: 0 })

  return getCacheHitRatio(totals)
}

export const getCacheUsageSeriesKey = (series: CacheUsageTimeSeries, index: number): string => (
  series.runtime || series.id || `agent-${index}`
)

export const buildCacheHitRatioChartData = (series: CacheUsageTimeSeries[]): CacheHitRatioChartRow[] => {
  const dates = new Map<string, CacheHitRatioChartRow>()
  series.forEach((agent, index) => {
    agent.metrics?.forEach(metric => {
      if (!metric.date) return
      const row = dates.get(metric.date) ?? { date: metric.date }
      const ratio = getCacheHitRatio(metric)
      if (ratio !== null) {
        row[getCacheUsageSeriesKey(agent, index)] = ratio
      }
      dates.set(metric.date, row)
    })
  })

  return Array.from(dates.values()).sort((a, b) => a.date.localeCompare(b.date))
}
