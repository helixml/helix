import { describe, expect, it } from 'vitest'

import {
  buildCacheHitRatioChartData,
  getAggregateCacheHitRatio,
  getCacheHitRatio,
  getTotalInputTokens,
  getUncachedInputTokens,
} from './usageMetrics'

describe('usage metrics', () => {
  it('calculates OpenAI cache hits from total input tokens', () => {
    const metric = {
      total_tokens: 1_200,
      completion_tokens: 200,
      cache_read_tokens: 800,
      cache_write_tokens: 0,
    }

    expect(getTotalInputTokens(metric)).toBe(1_000)
    expect(getUncachedInputTokens(metric)).toBe(200)
    expect(getCacheHitRatio(metric)).toBe(0.8)
  })

  it('handles historical Anthropic rows where prompt tokens excluded cache tokens', () => {
    const metric = {
      total_tokens: 1_300,
      completion_tokens: 200,
      cache_read_tokens: 800,
      cache_write_tokens: 200,
    }

    expect(getTotalInputTokens(metric)).toBe(1_100)
    expect(getUncachedInputTokens(metric)).toBe(100)
    expect(getCacheHitRatio(metric)).toBeCloseTo(800 / 1_100)
  })

  it('returns no ratio when there are no input tokens', () => {
    const metric = {
      total_tokens: 200,
      completion_tokens: 200,
      cache_read_tokens: 0,
    }

    expect(getCacheHitRatio(metric)).toBeNull()
  })

  it('weights aggregate hit ratios by input token volume', () => {
    const metrics = [
      { total_tokens: 1_000, completion_tokens: 0, cache_read_tokens: 1_000 },
      { total_tokens: 9_000, completion_tokens: 0, cache_read_tokens: 0 },
    ]

    expect(getAggregateCacheHitRatio(metrics)).toBe(0.1)
  })

  it('builds daily cache ratios for named and unassigned agents', () => {
    const chart = buildCacheHitRatioChartData([
      {
        runtime: 'claude_code',
        metrics: [
          { date: '2026-08-02T00:00:00Z', total_tokens: 1_000, completion_tokens: 100, cache_read_tokens: 810 },
          { date: '2026-08-03T00:00:00Z', total_tokens: 100, completion_tokens: 100, cache_read_tokens: 0 },
        ],
      },
      {
        metrics: [
          { date: '2026-08-02T00:00:00Z', total_tokens: 500, completion_tokens: 100, cache_read_tokens: 200 },
        ],
      },
    ])

    expect(chart).toEqual([
      { date: '2026-08-02T00:00:00Z', claude_code: 0.9, 'agent-1': 0.5 },
      { date: '2026-08-03T00:00:00Z' },
    ])
  })
})
