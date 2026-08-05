import { describe, expect, it } from 'vitest'

import { getTooltipStyleOverrides } from './theme'

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
