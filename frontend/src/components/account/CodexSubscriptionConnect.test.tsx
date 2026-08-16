import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CodexSubscriptionConnect, { CODEX_DEVICE_AUTH_URL } from './CodexSubscriptionConnect'

const startLoginMutate = vi.fn()
const deleteMutate = vi.fn()
let subscriptions: { id: string }[] = []

vi.mock('../../services/codexSubscriptionsService', () => ({
  codexSubscriptionsQueryKey: ['codex-subscriptions'],
  useCodexSubscriptions: () => ({ data: subscriptions }),
  useCreateCodexSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteCodexSubscription: () => ({ mutate: deleteMutate, isPending: false }),
  useStartCodexLogin: () => ({
    mutate: startLoginMutate,
    isPending: false,
  }),
  usePollCodexLogin: () => ({ data: undefined }),
}))

function renderConnect() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <CodexSubscriptionConnect />
    </QueryClientProvider>,
  )
}

describe('CodexSubscriptionConnect', () => {
  beforeEach(() => {
    subscriptions = []
    startLoginMutate.mockReset()
    deleteMutate.mockReset()
  })

  it('opens the dialog and shows Open ChatGPT immediately', () => {
    renderConnect()

    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(startLoginMutate).toHaveBeenCalledTimes(1)
    const openChatGPT = screen.getByRole('link', { name: 'Open ChatGPT' })
    expect(openChatGPT).toBeInTheDocument()
    expect(openChatGPT).toHaveAttribute('href', CODEX_DEVICE_AUTH_URL)
    expect(screen.getByText('Waiting for device code…')).toBeInTheDocument()
  })

  it('asks for confirmation before disconnecting', () => {
    subscriptions = [{ id: 'sub_codex_1' }]
    renderConnect()

    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    expect(deleteMutate).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toHaveTextContent('Disconnect ChatGPT Subscription')

    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }))
    expect(deleteMutate).not.toHaveBeenCalled()

    fireEvent.click(screen.getAllByRole('button', { name: 'Disconnect' })[0])
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Disconnect' }))
    expect(deleteMutate).toHaveBeenCalledWith('sub_codex_1', expect.any(Object))
  })
})
