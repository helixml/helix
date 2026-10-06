import { describe, expect, it } from 'vitest'
import { isOpaqueScriptError, isResizeObserverLoopError } from './mobileErrorNoise'

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

describe('isResizeObserverLoopError', () => {
  it('matches only the browser-generated notification', () => {
    expect(isResizeObserverLoopError('ResizeObserver loop completed with undelivered notifications.', '', 0, 0, null)).toBe(true)
    expect(isResizeObserverLoopError(
      'ResizeObserver loop completed with undelivered notifications.',
      '',
      0,
      0,
      'ResizeObserver loop completed with undelivered notifications.',
    )).toBe(true)
    expect(isResizeObserverLoopError('ResizeObserver loop completed with undelivered notifications')).toBe(false)
    expect(isResizeObserverLoopError('ResizeObserver loop limit exceeded')).toBe(false)
  })

  it('rejects matching messages with actionable provenance', () => {
    expect(isResizeObserverLoopError(
      'ResizeObserver loop completed with undelivered notifications.',
      'https://example.com/app.js',
      10,
      4,
      null,
    )).toBe(false)
    expect(isResizeObserverLoopError(
      'ResizeObserver loop completed with undelivered notifications.',
      '',
      0,
      0,
      new Error('ResizeObserver loop completed with undelivered notifications.'),
    )).toBe(false)
  })
})
