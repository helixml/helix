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

export interface ToolCallUsageMetric {
  tool_call_requests?: number
  tool_call_error_requests?: number
}

// A tool call error rate is only meaningful once enough requests have landed in
// the bucket. Below this a single bad call swings a day from 0% to 50%, which
// reads as a provider outage rather than the noise it is. Buckets under the
// floor are reported as null so the chart leaves a gap instead of a spike.
export const MIN_TOOL_CALL_REQUESTS = 20

export const getToolCallErrorRate = (
  metric: ToolCallUsageMetric,
  minRequests: number = MIN_TOOL_CALL_REQUESTS,
): number | null => {
  const requests = metric.tool_call_requests ?? 0
  if (requests < minRequests) return null
  return (metric.tool_call_error_requests ?? 0) / requests
}

export const getAggregateToolCallErrorRate = (metrics: ToolCallUsageMetric[]): number | null => {
  const totals = metrics.reduce((aggregate, metric) => ({
    tool_call_requests: aggregate.tool_call_requests + (metric.tool_call_requests ?? 0),
    tool_call_error_requests: aggregate.tool_call_error_requests + (metric.tool_call_error_requests ?? 0),
  }), { tool_call_requests: 0, tool_call_error_requests: 0 })

  // The aggregate is a ratio of sums, never a mean of daily ratios — the latter
  // would weight a day with three tool calls the same as a day with thirty
  // thousand. One request is enough to report a total.
  return getToolCallErrorRate(totals, 1)
}
