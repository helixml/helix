import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import AgentRestartRequiredBanner from './AgentRestartRequiredBanner'

describe('AgentRestartRequiredBanner', () => {
  it('renders nothing when no restart is pending', () => {
    render(<AgentRestartRequiredBanner visible={false} onRestart={vi.fn()} />)
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  it('renders when a restart is pending', () => {
    render(<AgentRestartRequiredBanner visible onRestart={vi.fn()} />)
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  // The restart discards the conversation, so it must never fire straight
  // off the banner button.
  it('does not restart until the cost is confirmed', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    expect(onRestart).not.toHaveBeenCalled()
    expect(screen.getByText(/current chat history is discarded/i)).toBeInTheDocument()
  })

  it('restarts once the dialog is confirmed', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    fireEvent.click(screen.getByTestId('agent-restart-confirm'))
    expect(onRestart).toHaveBeenCalledTimes(1)
  })

  it('cancelling the dialog restarts nothing', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    fireEvent.click(screen.getByTestId('agent-restart-cancel'))
    expect(onRestart).not.toHaveBeenCalled()
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('hides for this tab when dismissed', () => {
    render(<AgentRestartRequiredBanner visible onRestart={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /not now/i }))
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  // Live work in progress is the one thing a restart genuinely destroys.
  it('gates the restart while the agent is mid-turn', () => {
    render(<AgentRestartRequiredBanner visible working onRestart={vi.fn()} />)
    expect(screen.getByRole('button', { name: /restart sandbox/i })).toBeDisabled()
  })

  it('gates the restart while a lifecycle action is in flight', () => {
    render(<AgentRestartRequiredBanner visible busy onRestart={vi.fn()} />)
    expect(screen.getByRole('button', { name: /restart sandbox/i })).toBeDisabled()
  })
})
