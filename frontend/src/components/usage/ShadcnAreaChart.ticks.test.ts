import { describe, expect, it } from 'vitest'

import { niceStep, niceTicks } from './ShadcnAreaChart'

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
