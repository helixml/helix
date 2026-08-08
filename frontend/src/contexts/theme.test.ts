import { describe, expect, it } from 'vitest'

import { getDialogStyleTokens, getTooltipStyleOverrides } from './theme'

describe('tooltip theme', () => {
  it('uses the compact bordered surface in dark mode', () => {
    expect(getTooltipStyleOverrides(false)).toMatchObject({
      tooltip: {
        backgroundColor: '#1b1b1b',
        border: '1px solid #3a3a3a',
        borderRadius: '7px',
        color: '#f5f5f5',
        fontSize: '0.75rem',
        padding: '5px 8px',
      },
      arrow: {
        color: '#1b1b1b',
      },
    })
  })

  it('leaves light-mode tooltip styling to the MUI defaults', () => {
    expect(getTooltipStyleOverrides(true)).toEqual({
      tooltip: {},
      arrow: {},
    })
  })
})

describe('dialog theme', () => {
  it('lifts the dark dialog above the app background while preserving the glass treatment', () => {
    expect(getDialogStyleTokens(false, '#0a0a0a')).toEqual({
      surfaceFallback: '#191919',
      surface: 'color-mix(in srgb, color-mix(in srgb, #0a0a0a 94%, white) 80%, transparent)',
      surfaceFilter: 'blur(16px) saturate(1.08)',
      border: '1px solid rgba(255, 255, 255, 0.08)',
      shadow: 'inset 0 1px rgba(255, 255, 255, 0.04), 0 24px 72px -20px rgba(0, 0, 0, 0.90)',
      backdropFallback: 'rgba(0, 0, 0, 0.64)',
      backdrop: 'color-mix(in srgb, #0a0a0a 64%, transparent)',
    })
  })

  it('keeps the same separation treatment in light mode', () => {
    expect(getDialogStyleTokens(true, '#ffffff')).toMatchObject({
      surfaceFallback: '#ffffff',
      surface: 'color-mix(in srgb, #ffffff 80%, transparent)',
      surfaceFilter: 'blur(12px) saturate(1.14)',
      border: '1px solid rgba(0, 0, 0, 0.10)',
      backdropFallback: 'rgba(255, 255, 255, 0.60)',
      backdrop: 'color-mix(in srgb, #ffffff 60%, transparent)',
    })
  })
})
