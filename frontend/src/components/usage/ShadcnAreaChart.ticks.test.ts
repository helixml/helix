import { describe, expect, it } from 'vitest'

import { niceStep, niceTicks, plottedSeries } from './ShadcnAreaChart'

describe('niceTicks', () => {
  it('rounds axes to 1/2/2.5/5 × 10ⁿ steps', () => {
    expect(niceTicks(30.7e6)).toEqual([0, 10e6, 20e6, 30e6, 40e6])
    expect(niceTicks(3900)).toEqual([0, 1000, 2000, 3000, 4000])
    expect(niceTicks(0.13)).toEqual([0, 0.05, 0.1, 0.15])
    expect(niceTicks(2.1)).toEqual([0, 1, 2, 3])
  })

  it('gives an empty series a usable axis', () => {
    expect(niceTicks(0)).toEqual([0, 1])
    expect(niceStep(0)).toBe(1)
  })
})

describe('plottedSeries', () => {
  const series = [
    { key: 'a', label: 'A', color: '#000' },
    { key: 'zero', label: 'Zero', color: '#111' },
    { key: 'missing', label: 'Missing', color: '#222' },
  ]
  const data = [
    { date: '2026-11-01', a: 0, zero: 0 },
    { date: '2026-11-02', a: 3, zero: 0 },
  ]
  const keys = (v: Parameters<typeof plottedSeries>[2], zeroIsData: boolean) =>
    plottedSeries(series, data, v, zeroIsData).map(s => s.key)

  it('drops series with no positive value and ignores missing keys', () => {
    expect(keys('area', false)).toEqual(['a'])
    expect(keys('trend', false)).toEqual(['a'])
  })

  it('keeps an all-zero series when zero is data, except on a trend', () => {
    expect(keys('line', true)).toEqual(['a', 'zero'])
    expect(keys('trend', true)).toEqual(['a'])
  })
})
