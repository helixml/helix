import { fireEvent, render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it, vi } from 'vitest'

import ChatTurnNavigator from './ChatTurnNavigator'

const items = [
  { id: 'first', userText: 'First user message', assistantText: 'First assistant response' },
  { id: 'second', userText: 'Second user message', assistantText: 'Second assistant response' },
]

const renderNavigator = (onSelect = vi.fn()) => {
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: 'dark' } })}>
      <ChatTurnNavigator items={items} scrollContainer={null} onSelect={onSelect} />
    </ThemeProvider>,
  )
  return { button: screen.getByRole('button', { hidden: true }), onSelect }
}

describe('ChatTurnNavigator interactions', () => {
  it('previews the nearest turn under the pointer', () => {
    const { button } = renderNavigator()
    vi.spyOn(button, 'getBoundingClientRect').mockReturnValue({
      top: 100,
      bottom: 140,
      left: 0,
      right: 300,
      width: 300,
      height: 40,
      x: 0,
      y: 100,
      toJSON: () => ({}),
    })

    fireEvent.mouseMove(button, { clientY: 140 })

    expect(screen.getByText('Second user message')).toBeInTheDocument()
    expect(screen.getByText('Second assistant response')).toBeInTheDocument()
    expect(document.querySelector('[data-chat-turn-preview]')).toHaveStyle({ maxWidth: '300px' })
  })

  it('supports keyboard browsing and selection', () => {
    const { button, onSelect } = renderNavigator()

    fireEvent.focus(button)
    fireEvent.keyDown(button, { key: 'End' })
    expect(button).toHaveAttribute('aria-label', 'Jump to message: Second user message')

    fireEvent.keyDown(button, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith(items[1])
  })
})
