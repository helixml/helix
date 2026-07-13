import { fireEvent, render, screen } from '@testing-library/react'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import { CollapsibleToolCall } from './CollapsibleToolCall'

describe('CollapsibleToolCall', () => {
  it('marks disclosure growth so the chat keeps the header in place', () => {
    const { container } = render(
      <div data-session-scroll-container>
        <ThemeProvider theme={createTheme()}>
          <CollapsibleToolCall toolName="inspect_topic" status="Completed" body="topic details" />
        </ThemeProvider>
      </div>,
    )

    fireEvent.click(screen.getByText('inspect_topic'))

    expect(screen.getByText('topic details')).toBeInTheDocument()
    expect(container.firstElementChild).toHaveAttribute('data-preserve-disclosure-expansion', 'true')
  })
})
