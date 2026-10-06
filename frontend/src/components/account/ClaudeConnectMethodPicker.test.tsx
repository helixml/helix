import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ClaudeConnectMethodPicker from './ClaudeConnectMethodPicker'

describe('ClaudeConnectMethodPicker', () => {
  it('offers all three connect methods and marks the current one', () => {
    render(<ClaudeConnectMethodPicker value="oauth" onChange={() => {}} />)

    const tiles = screen.getAllByRole('radio')
    expect(tiles).toHaveLength(3)
    expect(tiles.map((t) => t.getAttribute('aria-label'))).toEqual([
      'Sign in with Claude',
      'Paste credentials',
      'Setup token',
    ])
    expect(screen.getByRole('radio', { name: 'Sign in with Claude' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
    expect(screen.getByRole('radio', { name: 'Setup token' })).toHaveAttribute(
      'aria-checked',
      'false',
    )
  })

  // The whole point of the tiles: the trade-offs are visible before you choose,
  // not discovered afterwards.
  it("states each method's lifetime, requirement and whether the account is knowable", () => {
    render(<ClaudeConnectMethodPicker value="oauth" onChange={() => {}} />)

    const setupToken = screen.getByRole('radio', { name: 'Setup token' })
    // Verified against Anthropic's docs: setup-token mints a one-year token.
    expect(setupToken).toHaveTextContent('A year')
    expect(setupToken).toHaveTextContent('Needs the Claude Code CLI')
    // Setup tokens lack the user:profile scope, so the account is unknowable.
    expect(setupToken).toHaveTextContent('Cannot show which account')

    const oauth = screen.getByRole('radio', { name: 'Sign in with Claude' })
    // Measured: refreshing does NOT extend the window, so the tile must not
    // promise an indefinite connection.
    expect(oauth).toHaveTextContent('About 9 days')
    expect(oauth).not.toHaveTextContent('Stays connected')
    expect(oauth).toHaveTextContent('Nothing to install')
    expect(oauth).toHaveTextContent('Shows the Claude account and plan')

    const credentials = screen.getByRole('radio', { name: 'Paste credentials' })
    expect(credentials).toHaveTextContent('Needs Claude Code signed in on your machine')
  })

  it('selects a method on click', () => {
    const onChange = vi.fn()
    render(<ClaudeConnectMethodPicker value="oauth" onChange={onChange} />)

    fireEvent.click(screen.getByRole('radio', { name: 'Setup token' }))
    expect(onChange).toHaveBeenCalledWith('setup_token')
  })

  // Tiles are divs, so keyboard activation only works because we handle it.
  it.each([['Enter'], [' ']])('selects a method with the %s key', (key) => {
    const onChange = vi.fn()
    render(<ClaudeConnectMethodPicker value="oauth" onChange={onChange} />)

    fireEvent.keyDown(screen.getByRole('radio', { name: 'Paste credentials' }), { key })
    expect(onChange).toHaveBeenCalledWith('credentials')
  })

  it('ignores other keys so typing does not change the selection', () => {
    const onChange = vi.fn()
    render(<ClaudeConnectMethodPicker value="oauth" onChange={onChange} />)

    fireEvent.keyDown(screen.getByRole('radio', { name: 'Setup token' }), { key: 'a' })
    expect(onChange).not.toHaveBeenCalled()
  })

  it('keeps every tile reachable by keyboard', () => {
    render(<ClaudeConnectMethodPicker value="oauth" onChange={() => {}} />)
    for (const tile of screen.getAllByRole('radio')) {
      expect(tile).toHaveAttribute('tabIndex', '0')
    }
  })
})
