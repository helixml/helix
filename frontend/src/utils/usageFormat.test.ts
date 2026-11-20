import { describe, expect, it } from 'vitest'
import { formatHours } from './usageFormat'

describe('formatHours', () => {
  it('reads short runtimes in minutes', () => {
    expect(formatHours(0)).toBe('0 h')
    expect(formatHours(0.001)).toBe('<1 min')
    expect(formatHours(0.25)).toBe('15 min')
  })

  it('reads longer runtimes in hours', () => {
    expect(formatHours(1.24)).toBe('1.2 h')
    expect(formatHours(972.98)).toBe('973 h')
  })
})
