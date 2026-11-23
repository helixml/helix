import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { buildSeriesChart, buildTypeChart } from './OrgUsage'
import { UsageProviderIcon } from '../usage/usageProviders'

describe('UsageProviderIcon', () => {
  it('uses the Helix logo for globally managed inference providers', () => {
    const { container } = render(<UsageProviderIcon provider="helix/ds4-flash-node06" size={18} />)

    const image = container.querySelector('img')
    expect(image).not.toBeNull()
    expect(image?.getAttribute('src')).toContain('logo.png')
    expect(image).toHaveStyle({ width: '18px', height: '18px' })
  })
})

describe('activity chart rows', () => {
  const day1 = { date: '2026-11-10', total_cost: 1.5, total_tokens: 1000, total_requests: 4, prompt_tokens: 900, cache_read_tokens: 600, cache_write_tokens: 100, completion_tokens: 100, prompt_cost: 0.5, cache_read_cost: 0.2, cache_write_cost: 0.3, completion_cost: 0.5 }
  const day2 = { ...day1, date: '2026-11-11', total_requests: 2 }

  it('plots the chosen metric per series, one row per date', () => {
    const rows = buildSeriesChart([
      { key: 'openrouter', metrics: [day2, day1] },
      { key: 'anthropic', metrics: [{ ...day1, total_requests: 7 }] },
    ], 'calls')
    expect(rows).toEqual([
      { date: '2026-11-10', openrouter: 4, anthropic: 7 },
      { date: '2026-11-11', openrouter: 2 },
    ])
    expect(buildSeriesChart([{ key: 'x', metrics: [day1] }], 'cost')[0].x).toBe(1.5)
  })

  it('splits tokens and cost into the same four types', () => {
    const [tokens] = buildTypeChart([day1], 'tokens')
    expect(tokens).toMatchObject({ input: 200, cacheRead: 600, cacheWrite: 100, output: 100 })
    const [cost] = buildTypeChart([day1], 'cost')
    expect(cost).toMatchObject({ input: 0.5, cacheRead: 0.2, cacheWrite: 0.3, output: 0.5 })
  })
})
