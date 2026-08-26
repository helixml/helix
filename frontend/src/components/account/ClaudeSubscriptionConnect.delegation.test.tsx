import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ClaudeSubscriptionConnect from './ClaudeSubscriptionConnect'

const delegationUpdate = vi.fn()
const organizations = [{
  id: 'org_1',
  name: 'Hello',
  memberships: [{ user_id: 'usr_me', role: 'owner' }],
}]

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    get: vi.fn(async () => [{
      id: 'sub_1',
      owner_type: 'user',
      owner_id: 'usr_me',
      name: 'My Claude Subscription',
      status: 'active',
      credential_type: 'oauth',
      access_token_expires_at: new Date(Date.now() + 3600_000).toISOString(),
      delegated_org_ids: [],
    }]),
    getApiClient: () => ({
      v1ClaudeSubscriptionsDelegationUpdate: delegationUpdate,
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    admin: false,
    user: { id: 'usr_me' },
    organizationTools: { organizations },
  }),
}))

function renderAccount() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ClaudeSubscriptionConnect variant="account" />
    </QueryClientProvider>,
  )
}

describe('ClaudeSubscriptionConnect delegation', () => {
  beforeEach(() => delegationUpdate.mockReset())

  it('enables subscription mode when sharing succeeds', async () => {
    delegationUpdate.mockResolvedValue({ data: {} })
    renderAccount()

    fireEvent.click(await screen.findByRole('checkbox', { name: 'Hello' }))

    await waitFor(() => expect(delegationUpdate).toHaveBeenCalledWith('sub_1', {
      delegated_org_ids: ['org_1'],
      switch_to_subscription: undefined,
    }))
  })

  it('asks before replacing API-provider mode', async () => {
    delegationUpdate
      .mockRejectedValueOnce({ response: { status: 409 } })
      .mockResolvedValueOnce({ data: {} })
    renderAccount()

    fireEvent.click(await screen.findByRole('checkbox', { name: 'Hello' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/currently uses API providers/i)).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Switch and share' }))
    await waitFor(() => expect(delegationUpdate).toHaveBeenLastCalledWith('sub_1', {
      delegated_org_ids: ['org_1'],
      switch_to_subscription: true,
    }))
  })
})
