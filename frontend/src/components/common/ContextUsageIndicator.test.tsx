import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  ContextUsageIndicator,
  SessionContextUsageIndicator,
} from './ContextUsageIndicator'

const useListInteractionsMock = vi.hoisted(() => vi.fn())

vi.mock('../../services/sessionService', () => ({
  useListInteractions: (...args: unknown[]) => useListInteractionsMock(...args),
}))

describe('ContextUsageIndicator', () => {
  it('renders context usage as an accessible circular progress indicator', async () => {
    render(<ContextUsageIndicator usedTokens={120_000} maxTokens={200_000} />)

    const indicator = screen.getByRole('progressbar', { name: 'Context window 60% used' })
    expect(indicator).toHaveAttribute('aria-valuenow', '60')

    fireEvent.mouseOver(indicator)
    expect(await screen.findByText('60% · 120k/200k')).toBeInTheDocument()
  })

  it('does not render without a valid context length', () => {
    const { container } = render(<ContextUsageIndicator usedTokens={100} maxTokens={0} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('keeps showing the latest completed usage while a new turn is waiting', () => {
    useListInteractionsMock.mockReturnValue({
      data: {
        data: {
          interactions: [
            { id: 'waiting', usage: {} },
            { id: 'complete', usage: { context_tokens: 120_000, context_length: 200_000 } },
          ],
        },
      },
    })

    render(<SessionContextUsageIndicator sessionId="ses_123" />)

    expect(screen.getByRole('progressbar', { name: 'Context window 60% used' })).toBeInTheDocument()
    expect(useListInteractionsMock).toHaveBeenCalledWith(
      'ses_123',
      0,
      5,
      'desc',
      { enabled: true, refetchInterval: 3000 },
    )
  })
})
