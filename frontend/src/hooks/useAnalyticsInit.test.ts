import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

let mockConfigData: any = undefined

vi.mock('../services/userService', () => ({
  useGetConfig: () => ({
    data: mockConfigData,
    isLoading: false,
    error: null,
  }),
}))

import useAnalyticsInit from './useAnalyticsInit'

describe('useAnalyticsInit', () => {
  beforeEach(() => {
    mockConfigData = undefined
    const win = window as any
    win.setUserFunctions = []
    win.viewPageFunctions = []
    win.emitErrorFunctions = []
    win.emitEventFunctions = []
    document.head.querySelectorAll('script').forEach(s => s.remove())
    win.dataLayer = undefined
  })

  it('does nothing when config is undefined', () => {
    renderHook(() => useAnalyticsInit())
    expect(document.head.querySelector('script')).toBeNull()
  })

  it('does nothing when config has no analytics values', () => {
    mockConfigData = { filestore_prefix: '', tools_enabled: true }
    renderHook(() => useAnalyticsInit())
    expect(document.head.querySelector('script')).toBeNull()
  })

  it('ignores legacy telemetry keys from an older server', () => {
    mockConfigData = {
      sentry_dsn_frontend: 'https://abc@sentry.io/123',
      rudderstack_write_key: 'write-key',
      rudderstack_data_plane_url: 'https://data.plane.url',
    }
    renderHook(() => useAnalyticsInit())

    const win = window as any
    expect(document.head.querySelector('script')).toBeNull()
    expect(win.setUserFunctions).toHaveLength(0)
    expect(win.viewPageFunctions).toHaveLength(0)
    expect(win.emitErrorFunctions).toHaveLength(0)
    expect(win.emitEventFunctions).toHaveLength(0)
  })

  describe('Google Analytics', () => {
    it('appends gtag script to head when google_analytics_frontend is set', () => {
      mockConfigData = { google_analytics_frontend: 'G-TEST12345' }
      renderHook(() => useAnalyticsInit())

      const script = document.head.querySelector('script[src*="gtag"]') as HTMLScriptElement
      expect(script).not.toBeNull()
      expect(script.src).toContain('G-TEST12345')
      expect(script.async).toBe(true)
    })

    it('populates dataLayer with config call', () => {
      mockConfigData = { google_analytics_frontend: 'G-TEST12345' }
      renderHook(() => useAnalyticsInit())

      const win = window as any
      expect(win.dataLayer).toBeDefined()
      expect(win.dataLayer.length).toBe(2) // 'js' + 'config'
    })

    it('only initializes once even if config reference changes', () => {
      mockConfigData = { google_analytics_frontend: 'G-TEST12345' }
      const { rerender } = renderHook(() => useAnalyticsInit())
      mockConfigData = { google_analytics_frontend: 'G-TEST12345' }
      rerender()

      expect(document.head.querySelectorAll('script[src*="gtag"]')).toHaveLength(1)
    })
  })
})
