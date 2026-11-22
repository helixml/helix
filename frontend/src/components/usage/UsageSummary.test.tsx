import { describe, expect, it } from 'vitest'

import { TypesUsageProviderKind } from '../../api/api'
import { buildDailyBars, enumerateDays, groupProviders } from './UsageSummary'
import { limitState } from './UsageLimits'
import { assignProviderColors } from './usageProviders'

describe('enumerateDays', () => {
  it('keeps a slot for every day in the range', () => {
    expect(enumerateDays('2026-11-28', '2026-12-02')).toEqual([
      '2026-11-28', '2026-11-29', '2026-11-30', '2026-12-01', '2026-12-02',
    ])
  })
})

describe('buildDailyBars', () => {
  it('stacks provider values per day and leaves quiet days empty', () => {
    const bars = buildDailyBars(['2026-11-20', '2026-11-21'], [
      { key: 'anthropic', metrics: [{ date: '2026-11-20T00:00:00Z', total_tokens: 10, total_cost: 1 }] },
      { key: 'pe_mine', metrics: [{ date: '2026-11-20T00:00:00Z', total_tokens: 5, total_cost: 2 }] },
    ], 'tokens')
    expect(bars[0]).toEqual({ day: '2026-11-20', total: 15, values: [{ key: 'anthropic', value: 10 }, { key: 'pe_mine', value: 5 }] })
    expect(bars[1].total).toBe(0)
  })
})

describe('groupProviders', () => {
  it('groups providers by who paid for them, largest group first', () => {
    const groups = groupProviders([
      { provider: 'anthropic', provider_kind: TypesUsageProviderKind.UsageProviderKindHelix, total_tokens: 10 },
      { provider: 'pe_a', provider_kind: TypesUsageProviderKind.UsageProviderKindAdded, total_tokens: 30 },
      { provider: 'pe_b', provider_kind: TypesUsageProviderKind.UsageProviderKindAdded, total_tokens: 40 },
      { provider: 'helix', provider_kind: TypesUsageProviderKind.UsageProviderKindLocal, total_tokens: 5 },
      { provider: 'legacy', total_tokens: 1 },
    ], 'tokens')
    expect(groups.map(group => group.kind)).toEqual([
      TypesUsageProviderKind.UsageProviderKindAdded,
      TypesUsageProviderKind.UsageProviderKindHelix,
      TypesUsageProviderKind.UsageProviderKindLocal,
      TypesUsageProviderKind.UsageProviderKindUnknown,
    ])
    expect(groups[0].total).toBe(70)
    expect(groups[0].rows.map(row => row.provider)).toEqual(['pe_b', 'pe_a'])
  })
})

describe('assignProviderColors', () => {
  it('never gives two providers of the same brand the same colour', () => {
    const colors = assignProviderColors([
      { provider: 'subscription/anthropic', name: 'Claude subscription' },
      { provider: 'anthropic', name: 'Anthropic' },
      { provider: 'pe_or1', name: 'OpenRouter' },
      { provider: 'pe_or2', name: 'user/openrouter' },
    ], false)
    expect(colors['subscription/anthropic']).toBe('#d97757')
    expect(new Set(Object.values(colors)).size).toBe(4)
  })
})

describe('limitState', () => {
  it('reads -1 as no limit and 0 as none allowed', () => {
    expect(limitState(3, -1).limited).toBe(false)
    expect(limitState(0, 0)).toMatchObject({ limited: true, tone: 'full' })
    expect(limitState(8, 10).tone).toBe('near')
    expect(limitState(2, 10).tone).toBe('normal')
  })
})
