import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

vi.mock('@sentry/react', () => ({
  init: vi.fn(),
  setUser: vi.fn(),
  captureException: vi.fn(),
  showReportDialog: vi.fn(),
  BrowserTracing: vi.fn(),
  Replay: vi.fn(),
}))

vi.mock('@rudderstack/analytics-js', () => ({
  RudderAnalytics: vi.fn().mockImplementation(() => ({
    load: vi.fn(),
    identify: vi.fn(),
    page: vi.fn(),
    track: vi.fn(),
  })),
}))

let mockConfigData: any = undefined

vi.mock('../services/userService', () => ({
  useGetConfig: () => ({
    data: mockConfigData,
    isLoading: false,
    error: null,
  }),
}))

import useAnalyticsInit from './useAnalyticsInit'
import * as Sentry from '@sentry/react'
import { RudderAnalytics } from '@rudderstack/analytics-js'

describe('useAnalyticsInit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConfigData = undefined
    const win = window as any
    win.setUserFunctions = []
    win.viewPageFunctions = []
    win.emitErrorFunctions = []
    win.emitEventFunctions = []
    document.head.querySelectorAll('script[src*="gtag"]').forEach(s => s.remove())
    win.dataLayer = undefined
  })

  it('does nothing when config is undefined', () => {
    mockConfigData = undefined
    renderHook(() => useAnalyticsInit())

    expect(Sentry.init).not.toHaveBeenCalled()
    expect(document.head.querySelector('script[src*="gtag"]')).toBeNull()
  })

  it('does nothing when config has no analytics values', () => {
    mockConfigData = { filestore_prefix: '', tools_enabled: true }
    renderHook(() => useAnalyticsInit())

    expect(Sentry.init).not.toHaveBeenCalled()
    expect(document.head.querySelector('script[src*="gtag"]')).toBeNull()
  })

  describe('Sentry', () => {
    it('initializes Sentry when sentry_dsn_frontend is set', () => {
      mockConfigData = { sentry_dsn_frontend: 'https://abc@sentry.io/123' }
      renderHook(() => useAnalyticsInit())

      expect(Sentry.init).toHaveBeenCalledWith(
        expect.objectContaining({ dsn: 'https://abc@sentry.io/123' })
      )
    })

    it('registers setUser callback for Sentry', () => {
      mockConfigData = { sentry_dsn_frontend: 'https://abc@sentry.io/123' }
      renderHook(() => useAnalyticsInit())

      const win = window as any
      expect(win.setUserFunctions.length).toBe(1)
      win.setUserFunctions[0]({ email: 'test@example.com', name: 'Test' })
      expect(Sentry.setUser).toHaveBeenCalledWith({ email: 'test@example.com', name: 'Test' })
    })

    it('registers emitError callback for Sentry', () => {
      mockConfigData = { sentry_dsn_frontend: 'https://abc@sentry.io/123' }
      renderHook(() => useAnalyticsInit())

      const win = window as any
      expect(win.emitErrorFunctions.length).toBe(1)
      const err = new Error('test')
      win.emitErrorFunctions[0](err)
      expect(Sentry.captureException).toHaveBeenCalledWith(err)
    })
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
  })

  describe('RudderStack', () => {
    it('initializes RudderStack when both keys are set', () => {
      mockConfigData = {
        rudderstack_write_key: 'write-key',
        rudderstack_data_plane_url: 'https://data.plane.url',
      }
      renderHook(() => useAnalyticsInit())

      const instance = (RudderAnalytics as any).mock.results[0].value
      expect(instance.load).toHaveBeenCalledWith('write-key', 'https://data.plane.url', {})
    })

    it('does not initialize RudderStack when only write_key is set', () => {
      mockConfigData = { rudderstack_write_key: 'write-key' }
      renderHook(() => useAnalyticsInit())

      expect(RudderAnalytics).not.toHaveBeenCalled()
    })

    it('registers user/page/event callbacks', () => {
      mockConfigData = {
        rudderstack_write_key: 'write-key',
        rudderstack_data_plane_url: 'https://data.plane.url',
      }
      renderHook(() => useAnalyticsInit())

      const win = window as any
      const instance = (RudderAnalytics as any).mock.results[0].value

      // setUser — strips token
      win.setUserFunctions[0]({ email: 'a@b.com', name: 'A', token: 'secret' })
      expect(instance.identify).toHaveBeenCalledWith('a@b.com', { email: 'a@b.com', name: 'A' })

      // viewPage
      win.viewPageFunctions[0]({ route: { name: 'home', path: '/' } })
      expect(instance.page).toHaveBeenCalledWith('home', '/', { name: 'home', path: '/' })

      // emitEvent
      win.emitEventFunctions[0]({ name: 'click', data: 1 })
      expect(instance.track).toHaveBeenCalledWith('click', { name: 'click', data: 1 })
    })
  })

  describe('initialization guard', () => {
    it('only initializes once even if config reference changes', () => {
      mockConfigData = { sentry_dsn_frontend: 'https://abc@sentry.io/123' }
      const { rerender } = renderHook(() => useAnalyticsInit())

      expect(Sentry.init).toHaveBeenCalledTimes(1)

      mockConfigData = { sentry_dsn_frontend: 'https://abc@sentry.io/123' }
      rerender()

      expect(Sentry.init).toHaveBeenCalledTimes(1)
    })
  })
})
