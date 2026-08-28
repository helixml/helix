import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SessionPromptQueue from './SessionPromptQueue'

const mocks = vi.hoisted(() => ({
  deletePrompt: vi.fn(),
  listPrompts: vi.fn(),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({ v1PromptHistoryDelete: mocks.deletePrompt }),
  }),
}))

vi.mock('../../services/promptHistoryService', () => ({
  listSessionPromptHistory: (...args: unknown[]) => mocks.listPrompts(...args),
}))

describe('SessionPromptQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.deletePrompt.mockResolvedValue({})
    mocks.listPrompts.mockResolvedValue({
      entries: [{ id: 'prompt-1', status: 'sending', content: 'stuck prompt' }],
    })
  })

  it('deletes a stuck prompt and refreshes the queue', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <SessionPromptQueue sessionId="session-1" />
      </QueryClientProvider>,
    )

    await screen.findByText('stuck prompt')
    fireEvent.click(screen.getByRole('button', { name: 'Remove from queue' }))

    await waitFor(() => expect(mocks.deletePrompt).toHaveBeenCalledWith('prompt-1'))
    await waitFor(() => expect(mocks.listPrompts).toHaveBeenCalledTimes(2))
  })
})
