import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ClaudeSubscriptionConnect from './ClaudeSubscriptionConnect'

// The list endpoint returns the user's own subscriptions first, then appends
// the subscriptions of every organisation they belong to. The button variant
// used to act on subscriptions[0], so a user with no personal subscription
// operated on an org's row without ever being told.
const deleteSubscription = vi.fn()

let subscriptions: Array<Record<string, unknown>> = []

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    get: vi.fn(async () => subscriptions),
    post: vi.fn(async () => null),
    delete: vi.fn(async () => null),
    getApiClient: () => ({
      v1ClaudeSubscriptionsCreate: vi.fn(),
      v1ClaudeSubscriptionsDelete: deleteSubscription,
      v1ClaudeSubscriptionsOauthStartCreate: vi.fn(),
      v1ClaudeSubscriptionsOauthCompleteCreate: vi.fn(),
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn(), setSnackbar: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    admin: false,
    user: { id: 'usr_me' },
    organizationTools: { organizations: [] },
  }),
}))

const orgSub = {
  id: 'csub_org',
  owner_type: 'org',
  owner_id: 'org_shared',
  name: 'Shared Claude Subscription',
  status: 'active',
  credential_type: 'oauth',
  access_token_expires_at: new Date(Date.now() + 3600_000).toISOString(),
}

const personalSub = {
  id: 'csub_mine',
  owner_type: 'user',
  owner_id: 'usr_me',
  name: 'My Claude Subscription',
  status: 'active',
  credential_type: 'oauth',
  access_token_expires_at: new Date(Date.now() + 3600_000).toISOString(),
}

async function renderButton(props: Record<string, unknown> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ClaudeSubscriptionConnect variant="button" {...props} />
    </QueryClientProvider>,
  )
  return result
}

// The row button and its confirmation dialog share the "Disconnect" label, so
// open the dialog first and confirm inside it.
async function confirmDisconnect() {
  fireEvent.click(await screen.findByRole('button', { name: 'Disconnect' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }))
}

describe('ClaudeSubscriptionConnect — which subscription the button acts on', () => {
  beforeEach(() => {
    deleteSubscription.mockReset()
    subscriptions = []
  })

  it('offers to connect when the only subscription belongs to an org', async () => {
    // The regression: this said "Disconnect", contradicting the card around it,
    // which correctly reads "not connected" — and left no way to connect one.
    subscriptions = [orgSub]
    await renderButton()

    expect(await screen.findByRole('button', { name: 'Connect' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument()
  })

  it('never offers to disconnect an org subscription from a personal control', async () => {
    subscriptions = [orgSub]
    await renderButton()

    fireEvent.click(await screen.findByRole('button', { name: 'Connect' }))
    expect(deleteSubscription).not.toHaveBeenCalled()
  })

  it('acts on the personal subscription when there is one', async () => {
    // Own row first, org row appended — the ordering that made [0] look right.
    subscriptions = [personalSub, orgSub]
    await renderButton()

    await confirmDisconnect()
    await waitFor(() => expect(deleteSubscription).toHaveBeenCalledWith(personalSub.id))
  })

  it('acts on the org subscription when asked to manage that org', async () => {
    // With orgId the control is explicitly the org's, so the org row is right.
    subscriptions = [personalSub, orgSub]
    await renderButton({ orgId: 'org_shared' })

    await confirmDisconnect()
    await waitFor(() => expect(deleteSubscription).toHaveBeenCalledWith(orgSub.id))
  })

  it('offers to connect when the named org has no subscription', async () => {
    subscriptions = [personalSub]
    await renderButton({ orgId: 'org_without_one' })

    expect(await screen.findByRole('button', { name: 'Connect' })).toBeInTheDocument()
  })
})
