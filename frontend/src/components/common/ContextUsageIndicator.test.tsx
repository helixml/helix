import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  ContextUsageIndicator,
  SessionContextUsageIndicator,
} from './ContextUsageIndicator'

const useListInteractionsMock = vi.hoisted(() => vi.fn())
const useGetSessionExecutionConfigMock = vi.hoisted(() => vi.fn())

vi.mock('../../services/sessionService', () => ({
  useListInteractions: (...args: unknown[]) => useListInteractionsMock(...args),
  useGetSessionExecutionConfig: (...args: unknown[]) => useGetSessionExecutionConfigMock(...args) ?? {},
}))

describe('ContextUsageIndicator', () => {
  beforeEach(() => {
    useListInteractionsMock.mockReset()
    useGetSessionExecutionConfigMock.mockReset()
  })

  it('renders context usage as an accessible circular progress indicator', async () => {
    render(
      <ContextUsageIndicator
        usedTokens={120_000}
        maxTokens={200_000}
        totalProcessedTokens={7_800_000}
        compactionAgentName="Codex"
      />,
    )

    const indicator = screen.getByRole('progressbar', { name: 'Context window 60% used' })
    expect(indicator).toHaveAttribute('aria-valuenow', '60')

    fireEvent.mouseOver(indicator)
    expect(await screen.findByText('60% · 120k/200k')).toBeInTheDocument()
    expect(screen.getByText('7.8m')).toBeInTheDocument()
    expect(screen.getByText('Codex automatically compacts its context when needed.')).toBeInTheDocument()
  })

  it('renders an empty meter until context usage is available', async () => {
    render(<ContextUsageIndicator usedTokens={100} maxTokens={0} />)

    const indicator = screen.getByRole('img', { name: 'Context window usage unavailable' })
    expect(indicator).not.toHaveAttribute('aria-valuenow')

    fireEvent.mouseOver(indicator)
    expect(await screen.findByText('Available after the next agent response')).toBeInTheDocument()
  })

  it('renders an empty meter before a session has any usage snapshots', () => {
    useListInteractionsMock.mockReturnValue({ data: { data: { interactions: [] } } })

    render(<SessionContextUsageIndicator sessionId="ses_new" />)

    expect(screen.getByRole('img', { name: 'Context window usage unavailable' })).toBeInTheDocument()
  })

  it('keeps showing the latest completed usage while a new turn is waiting', () => {
    useGetSessionExecutionConfigMock.mockReturnValue({ data: { runtime: 'codex_cli' } })
    useListInteractionsMock.mockReturnValue({
      data: {
        data: {
          interactions: [
            { id: 'waiting', usage: {} },
            {
              id: 'complete',
              usage: {
                context_tokens: 120_000,
                context_length: 200_000,
                total_processed_tokens: 7_800_000,
              },
            },
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
