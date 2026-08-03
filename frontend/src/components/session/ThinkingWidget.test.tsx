import { fireEvent, render, screen } from '@testing-library/react'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import ThinkingWidget from './ThinkingWidget'

const renderWidget = (isStreaming = false) => render(
  <ThemeProvider theme={createTheme()}>
    <ThinkingWidget text={"Inspect the current\nsubscription state"} isStreaming={isStreaming} />
  </ThemeProvider>,
)

describe('ThinkingWidget', () => {
  it('uses a collapsed disclosure and reveals the thought on click', () => {
    const { container } = render(
      <div data-session-scroll-container>
        <ThemeProvider theme={createTheme()}>
          <ThinkingWidget text={"Inspect the current\nsubscription state"} isStreaming={false} />
        </ThemeProvider>
      </div>,
    )

    expect(screen.getByText('Thoughts')).toBeInTheDocument()
    expect(screen.queryByText('Inspect the current subscription state')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('Thoughts'))

    expect(container.querySelector('[data-session-scroll-container]'))
      .toHaveAttribute('data-preserve-disclosure-expansion', 'true')
    expect(screen.getByText('Inspect the current subscription state')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Collapse thoughts' })).toBeInTheDocument()
  })

  it('shows single-line thoughts inline without a second disclosure', () => {
    render(
      <ThemeProvider theme={createTheme()}>
        <ThinkingWidget text="Check the current branch" isStreaming={false} />
      </ThemeProvider>,
    )

    expect(screen.getByText('Thoughts')).toBeInTheDocument()
    expect(screen.getByText('Check the current branch')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Expand thoughts' })).not.toBeInTheDocument()
  })

  it('shows an active thinking status while streaming', () => {
    renderWidget(true)

    expect(screen.getByText(/^Thinking \d+:\d{2}$/)).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toBeInTheDocument()
  })
})
