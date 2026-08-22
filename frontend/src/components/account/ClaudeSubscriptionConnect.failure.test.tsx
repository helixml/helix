import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ClaudeSubscriptionConnect from './ClaudeSubscriptionConnect'

// The bug this file exists for: useApi().post catches errors, shows its own
// snackbar and resolves with null instead of throwing. The connect handler
// awaited it, so a failed request fell straight through to the success path —
// green "connected" toast, dialog closed, pasted credentials wiped, no error
// shown. Going through the generated client (raw axios, which rejects) is what
// makes the catch reachable.
const createSubscription = vi.fn()
const deleteSubscription = vi.fn()
const snackbarSuccess = vi.fn()

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    get: vi.fn(async () => []),
    post: vi.fn(async () => null),
    delete: vi.fn(async () => null),
    getApiClient: () => ({
      v1ClaudeSubscriptionsCreate: createSubscription,
      v1ClaudeSubscriptionsDelete: deleteSubscription,
      v1ClaudeSubscriptionsOauthStartCreate: vi.fn(),
      v1ClaudeSubscriptionsOauthCompleteCreate: vi.fn(),
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: snackbarSuccess, error: vi.fn(), setSnackbar: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({ admin: false, user: { id: 'usr_1' }, organizationTools: { organizations: [] } }),
}))

function renderConnect() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ClaudeSubscriptionConnect variant="button" />
    </QueryClientProvider>,
  )
}

async function openDialogOnSetupToken() {
  fireEvent.click(await screen.findByRole('button', { name: /connect/i }))
  fireEvent.click(await screen.findByRole('radio', { name: 'Setup token' }))
}

describe('ClaudeSubscriptionConnect — connect failures', () => {
  beforeEach(() => {
    createSubscription.mockReset()
    deleteSubscription.mockReset()
    snackbarSuccess.mockReset()
  })

  it('reports a rejected setup token instead of claiming success', async () => {
    createSubscription.mockRejectedValue({
      response: { data: { error: 'invalid or expired token (401 from Anthropic)' } },
    })

    renderConnect()
    await openDialogOnSetupToken()

    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 'r'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await waitFor(() => expect(createSubscription).toHaveBeenCalled())
    // The failure must surface, and must not be dressed up as a success.
    await screen.findByText(/invalid or expired token/i)
    expect(snackbarSuccess).not.toHaveBeenCalled()
    // The dialog stays open so the pasted token is not thrown away.
    expect(screen.getByRole('button', { name: /^Connect$/ })).toBeInTheDocument()
  })

  it('confirms success only when the request actually succeeded', async () => {
    createSubscription.mockResolvedValue({ data: { id: 'csub_1' } })

    renderConnect()
    await openDialogOnSetupToken()

    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 'g'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await waitFor(() => expect(snackbarSuccess).toHaveBeenCalledWith('Claude subscription connected'))
  })
})
