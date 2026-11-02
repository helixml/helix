import { describe, it, expect } from 'vitest'

import {
  defaultSandboxResolution,
  deriveDisplaySettings,
  sandboxResolution,
  sandboxResolutionLabel,
  sandboxResolutions,
} from './externalAgentDisplay'

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

// Helix for Mac on a MacBook Pro set to "More Space" (1800pt at 2x).
const mac = { width: 3600, height: 2024, zoom_level: 200 }

describe('native display', () => {
  it('is the default where the host reported its display', () => {
    expect(defaultSandboxResolution(mac)).toBe('native')
    expect(deriveDisplaySettings(undefined, mac)).toEqual({ width: 3600, height: 2024, fps: 60 })
    expect(deriveDisplaySettings({}, mac)).toEqual({ width: 3600, height: 2024, fps: 60 })
    expect(sandboxResolutionLabel(undefined, mac)).toBe('Native')
  })

  it('is offered first in the menu, with presets still selectable', () => {
    const options = sandboxResolutions(mac)
    expect(options.map((o) => o.value)).toEqual(['native', '1080p', '4k', '5k'])
    expect(options[0].description).toBe('3600×2024 · matches this Mac')
    expect(deriveDisplaySettings({ resolution: '1080p' }, mac)).toEqual({ width: 1920, height: 1080, fps: 60 })
  })

  it('is absent elsewhere, so a saved native setting falls back to 1080p', () => {
    expect(sandboxResolutions(undefined).map((o) => o.value)).toEqual(['1080p', '4k', '5k'])
    expect(defaultSandboxResolution({})).toBe('1080p')
    expect(sandboxResolution({ resolution: 'native' })).toBe('1080p')
    expect(deriveDisplaySettings({ resolution: 'native' })).toEqual({ width: 1920, height: 1080, fps: 60 })
  })
})
