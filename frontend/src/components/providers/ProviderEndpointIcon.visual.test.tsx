import { render } from '@testing-library/react'
import { createTheme, hexToRgb, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import ProviderEndpointIcon, { ProviderMark } from './ProviderEndpointIcon'
import { PROVIDERS } from './types'

describe('ProviderEndpointIcon theme colors', () => {
  it.each([
    ['known provider', { name: 'anthropic' }],
    ['unknown provider', { name: 'private-model-host' }],
  ])('uses light theme foreground for a %s', (_, endpoint) => {
    const theme = createTheme({ palette: { mode: 'light' } })
    const { container } = render(
      <ThemeProvider theme={theme}>
        <ProviderEndpointIcon endpoint={endpoint} />
      </ThemeProvider>,
    )

    const iconContainer = container.querySelector('svg')?.parentElement
    expect(iconContainer).not.toBeNull()
    expect(getComputedStyle(iconContainer!).color).toBe(theme.palette.text.primary)
  })

  it.each(['light', 'dark'] as const)('gives current-color provider marks a visible %s theme foreground', (mode) => {
    const theme = createTheme({ palette: { mode } })
    const anthropic = PROVIDERS.find(provider => provider.id === 'user/anthropic')!
    const { container } = render(
      <ThemeProvider theme={theme}>
        <ProviderMark provider={anthropic} />
      </ThemeProvider>,
    )

    const mark = container.querySelector('svg')?.parentElement
    expect(mark).not.toBeNull()
    const expectedColor = theme.palette.text.primary.startsWith('#')
      ? hexToRgb(theme.palette.text.primary)
      : theme.palette.text.primary
    expect(getComputedStyle(mark!).color).toBe(expectedColor)
  })
})
