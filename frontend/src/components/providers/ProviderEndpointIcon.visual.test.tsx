import { render } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import ProviderEndpointIcon from './ProviderEndpointIcon'

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
})
