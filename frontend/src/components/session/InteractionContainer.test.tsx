import { render } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import InteractionContainer from './InteractionContainer'

const darkTheme = createTheme({ palette: { mode: 'dark' } })

const renderMessage = (messageRole: 'user' | 'assistant') => render(
  <ThemeProvider theme={darkTheme}>
    <InteractionContainer messageRole={messageRole}>Message</InteractionContainer>
  </ThemeProvider>,
)

describe('InteractionContainer chat typography', () => {
  it('uses T3 foreground opacity for assistant messages', () => {
    const { container } = renderMessage('assistant')
    const message = container.querySelector('[data-chat-message-role="assistant"]')

    expect(message).toHaveStyle({ color: 'rgba(245, 245, 245, 0.8)' })
  })

  it('keeps user messages at full foreground opacity', () => {
    const { container } = renderMessage('user')
    const message = container.querySelector('[data-chat-message-role="user"]')

    expect(message).toHaveStyle({ color: '#f5f5f5' })
  })
})
