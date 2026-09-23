import { describe, expect, it } from 'vitest'
import { isMobileErrorNoise } from './mobileErrorNoise'

describe('isMobileErrorNoise', () => {
  it('ignores opaque cross-origin script errors from mobile browsers', () => {
    expect(isMobileErrorNoise('Script error.')).toBe(true)
    expect(isMobileErrorNoise('Script error')).toBe(true)
  })

  it('does not hide actionable application errors', () => {
    expect(isMobileErrorNoise('TypeError: undefined is not an object')).toBe(false)
    expect(isMobileErrorNoise('Script error while loading application state')).toBe(false)
  })
})
