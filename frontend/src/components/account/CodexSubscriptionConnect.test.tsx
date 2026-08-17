import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CodexSubscriptionConnect, { CODEX_DEVICE_AUTH_URL } from './CodexSubscriptionConnect'

const startLoginMutate = vi.fn()
const cancelLoginMutate = vi.fn()
const deleteMutate = vi.fn()
let subscriptions: { id: string; owner_type: 'user' | 'org'; owner_id: string }[] = []
let loginStatus: { found?: boolean; code?: string; url?: string } | undefined

vi.mock('../../services/codexSubscriptionsService', () => ({
  codexSubscriptionsQueryKey: ['codex-subscriptions'],
  useCodexSubscriptions: () => ({ data: subscriptions }),
  useCreateCodexSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteCodexSubscription: () => ({ mutate: deleteMutate, isPending: false }),
  useStartCodexLogin: () => ({
    mutate: startLoginMutate,
    isPending: false,
  }),
  useCancelCodexLogin: () => ({ mutate: cancelLoginMutate, isPending: false }),
  usePollCodexLogin: () => ({ data: loginStatus }),
}))

function renderConnect(orgId?: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <CodexSubscriptionConnect orgId={orgId} />
    </QueryClientProvider>,
  )
}

describe('CodexSubscriptionConnect', () => {
  beforeEach(() => {
    subscriptions = []
    loginStatus = undefined
    startLoginMutate.mockReset()
    cancelLoginMutate.mockReset()
    deleteMutate.mockReset()
  })

  it('opens with explicit generated and manual alternatives', () => {
    renderConnect()

    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(startLoginMutate).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Generate code' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enter credentials' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Continue to ChatGPT' })).not.toBeInTheDocument()
  })

  it('does not offer ChatGPT until a generated device code exists', () => {
    renderConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Generate code' }))

    expect(startLoginMutate).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Waiting for device code…')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Continue to ChatGPT' })).not.toBeInTheDocument()
  })

  it('shows the code before linking to ChatGPT', () => {
    loginStatus = { found: false, code: '2E5J-JKA6Q', url: CODEX_DEVICE_AUTH_URL }
    startLoginMutate.mockImplementation((_value, options) => options.onSuccess({ session_id: 'ses_login' }))
    renderConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Generate code' }))

    expect(screen.getByText('2E5J-JKA6Q')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Continue to ChatGPT' }))
      .toHaveAttribute('href', CODEX_DEVICE_AUTH_URL)
  })

  it('opens the manual auth.json form without starting a sandbox', () => {
    renderConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Enter credentials' }))

    expect(startLoginMutate).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Codex auth.json')).toBeInTheDocument()
  })

  it('cancels the generated login sandbox when the dialog closes', () => {
    startLoginMutate.mockImplementation((_value, options) => options.onSuccess({ session_id: 'ses_login' }))
    renderConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Generate code' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(cancelLoginMutate).toHaveBeenCalledWith('ses_login')
  })

  it('asks for confirmation before disconnecting', () => {
    subscriptions = [{ id: 'sub_codex_1', owner_type: 'user', owner_id: 'user_1' }]
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

  it('does not treat another owner subscription as the current connection', () => {
    subscriptions = [{ id: 'sub_org_1', owner_type: 'org', owner_id: 'org_1' }]

    renderConnect()

    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument()
  })
})
