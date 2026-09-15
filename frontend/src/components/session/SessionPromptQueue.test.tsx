import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SessionPromptQueue from './SessionPromptQueue'
import { useSessionPromptQueue } from './useSessionPromptQueue'

const mocks = vi.hoisted(() => ({
  deletePrompt: vi.fn(),
  listPrompts: vi.fn(),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({ v1PromptHistoryDelete: mocks.deletePrompt, v1SessionsRestartAgentCreate: vi.fn() }),
  }),
}))

vi.mock('../../services/promptHistoryService', () => ({
  listSessionPromptHistory: (...args: unknown[]) => mocks.listPrompts(...args),
}))

// The composer renders the queue from the hook exactly like AgentChat does.
const Harness = ({ busy = true }: { busy?: boolean }) => {
  const queue = useSessionPromptQueue('session-1', busy)
  return (
    <SessionPromptQueue
      sessionId="session-1"
      entries={queue.entries}
      onRemove={queue.remove}
      onRestartAgent={queue.restartAgent}
    />
  )
}

const renderHarness = (busy?: boolean) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <Harness busy={busy} />
    </QueryClientProvider>,
  )
}

describe('SessionPromptQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.deletePrompt.mockResolvedValue({})
    mocks.listPrompts.mockResolvedValue({
      entries: [
        { id: 'prompt-1', status: 'pending', content: 'waiting prompt', created_at: '2026-09-07T00:00:00Z' },
        // Already handed to the agent: it is the turn on screen, not queued.
        { id: 'prompt-2', status: 'sending', content: 'in-flight prompt', created_at: '2026-09-07T00:00:01Z' },
      ],
    })
  })

  it('lists waiting prompts, hides the in-flight one, and removes a prompt then refreshes', async () => {
    renderHarness()

    await screen.findByText('waiting prompt')
    expect(screen.queryByText('in-flight prompt')).not.toBeInTheDocument()
    expect(screen.getByText('1 queued')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove from queue' }))

    await waitFor(() => expect(mocks.deletePrompt).toHaveBeenCalledWith('prompt-1'))
    await waitFor(() => expect(mocks.listPrompts).toHaveBeenCalledTimes(2))
  })

  it('offers Restart for a crashed prompt', async () => {
    mocks.listPrompts.mockResolvedValue({
      entries: [{ id: 'prompt-3', status: 'failed', content: 'crashed prompt', error_message: 'Claude Agent process exited unexpectedly', retry_count: 4, created_at: '2026-09-07T00:00:00Z' }],
    })
    renderHarness()
    await screen.findByText('crashed prompt')
    expect(screen.getByRole('button', { name: /Restart/ })).toBeInTheDocument()
  })
})
