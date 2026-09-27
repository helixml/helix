import { describe, expect, it } from 'vitest'
import { isOpaqueScriptError } from './mobileErrorNoise'

describe('isOpaqueScriptError', () => {
  it('identifies script errors with opaque window.onerror metadata', () => {
    expect(isOpaqueScriptError('Script error.', '', 0, 0, null)).toBe(true)
    expect(isOpaqueScriptError('Script error', undefined, undefined, undefined, undefined)).toBe(true)
  })

  it('rejects matching messages with actionable provenance', () => {
    expect(isOpaqueScriptError('Script error.', 'https://example.com/app.js', 10, 4, null)).toBe(false)
    expect(isOpaqueScriptError('Script error.', '', 0, 0, new Error('Script error.'))).toBe(false)
    expect(isOpaqueScriptError('Script error while loading application state', '', 0, 0, null)).toBe(false)
  })
})
