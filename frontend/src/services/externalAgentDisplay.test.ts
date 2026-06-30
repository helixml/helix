import { describe, it, expect } from 'vitest'

import { deriveDisplaySettings } from './externalAgentDisplay'
import { TypesApp } from '../api/api'

const appWith = (cfg: Record<string, unknown>): TypesApp =>
  ({ config: { helix: { external_agent_config: cfg } } } as unknown as TypesApp)

describe('deriveDisplaySettings', () => {
  it('falls back to 1920x1080x60 when app is undefined', () => {
    expect(deriveDisplaySettings(undefined)).toEqual({ width: 1920, height: 1080, fps: 60 })
  })

  it('falls back when there is no external_agent_config', () => {
    expect(deriveDisplaySettings({ config: { helix: {} } } as unknown as TypesApp)).toEqual({
      width: 1920,
      height: 1080,
      fps: 60,
    })
  })

  it('uses explicit dimensions and refresh rate', () => {
    expect(deriveDisplaySettings(appWith({ display_width: 1280, display_height: 720, display_refresh_rate: 30 }))).toEqual({
      width: 1280,
      height: 720,
      fps: 30,
    })
  })

  it('honours the 4k preset over explicit dimensions', () => {
    expect(deriveDisplaySettings(appWith({ resolution: '4k', display_width: 800, display_height: 600 }))).toEqual({
      width: 3840,
      height: 2160,
      fps: 60,
    })
  })

  it('honours the 5k preset', () => {
    expect(deriveDisplaySettings(appWith({ resolution: '5k' }))).toEqual({
      width: 5120,
      height: 2880,
      fps: 60,
    })
  })

  it('honours the 1080p preset', () => {
    expect(deriveDisplaySettings(appWith({ resolution: '1080p' }))).toEqual({
      width: 1920,
      height: 1080,
      fps: 60,
    })
  })
})
