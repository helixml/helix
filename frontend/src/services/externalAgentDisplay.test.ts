import { describe, it, expect } from 'vitest'

import { deriveDisplaySettings, sandboxResolutionLabel } from './externalAgentDisplay'

describe('deriveDisplaySettings', () => {
  it('falls back to 1920x1080x60 when the task has no display', () => {
    expect(deriveDisplaySettings(undefined)).toEqual({ width: 1920, height: 1080, fps: 60 })
    expect(deriveDisplaySettings({})).toEqual({ width: 1920, height: 1080, fps: 60 })
  })

  it('resolves the 4k preset', () => {
    expect(deriveDisplaySettings({ resolution: '4k' })).toEqual({ width: 3840, height: 2160, fps: 60 })
  })

  it('resolves the 5k preset', () => {
    expect(deriveDisplaySettings({ resolution: '5k' })).toEqual({ width: 5120, height: 2880, fps: 60 })
  })

  it('treats unknown presets as 1080p', () => {
    expect(deriveDisplaySettings({ resolution: '8k' })).toEqual({ width: 1920, height: 1080, fps: 60 })
  })
})

describe('sandboxResolutionLabel', () => {
  it('labels presets', () => {
    expect(sandboxResolutionLabel(undefined)).toBe('1080p')
    expect(sandboxResolutionLabel({ resolution: '4k' })).toBe('4K')
  })
})
