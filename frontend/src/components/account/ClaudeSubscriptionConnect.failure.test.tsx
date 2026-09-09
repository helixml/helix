import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ClaudeSubscriptionConnect from './ClaudeSubscriptionConnect'
import { codeAgentHarnessesQueryKey } from '../../services/codeAgentHarnessesService'

// The bug this file exists for: useApi().post catches errors, shows its own
// snackbar and resolves with null instead of throwing. The connect handler
// awaited it, so a failed request fell straight through to the success path —
// green "connected" toast, dialog closed, pasted credentials wiped, no error
// shown. Going through the generated client (raw axios, which rejects) is what
// makes the catch reachable.
const createSubscription = vi.fn()
const deleteSubscription = vi.fn()
const delegationUpdate = vi.fn()
const oauthStart = vi.fn()
const oauthComplete = vi.fn()
const snackbarSuccess = vi.fn()

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    get: vi.fn(async () => []),
    post: vi.fn(async () => null),
    delete: vi.fn(async () => null),
    getApiClient: () => ({
      v1ClaudeSubscriptionsCreate: createSubscription,
      v1ClaudeSubscriptionsDelete: deleteSubscription,
      v1ClaudeSubscriptionsDelegationUpdate: delegationUpdate,
      v1ClaudeSubscriptionsOauthStartCreate: oauthStart,
      v1ClaudeSubscriptionsOauthCompleteCreate: oauthComplete,
    }),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: snackbarSuccess, error: vi.fn(), setSnackbar: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    admin: false,
    user: { id: 'usr_1' },
    organizationTools: {
      organizations: [{ id: 'org_target', name: 'acme', display_name: 'Acme' }],
    },
  }),
}))

function renderConnect(props: { enableForOrgId?: string; onConnected?: () => void } = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <ClaudeSubscriptionConnect variant="button" {...props} />
      </QueryClientProvider>,
    ),
  }
}

async function openDialogOnSetupToken() {
  fireEvent.click(await screen.findByRole('button', { name: /connect/i }))
  fireEvent.click(await screen.findByRole('radio', { name: 'Setup token' }))
}

describe('ClaudeSubscriptionConnect — connect failures', () => {
  beforeEach(() => {
    createSubscription.mockReset()
    deleteSubscription.mockReset()
    delegationUpdate.mockReset()
    oauthStart.mockReset()
    oauthComplete.mockReset()
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

  it('shares a setup-token subscription connected from organization settings', async () => {
    createSubscription.mockResolvedValue({
      data: { id: 'csub_setup', delegated_org_ids: ['org_existing'] },
    })
    delegationUpdate.mockResolvedValue({ data: {} })

    renderConnect({ enableForOrgId: 'org_target' })
    await openDialogOnSetupToken()
    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 's'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await waitFor(() => expect(delegationUpdate).toHaveBeenCalledWith('csub_setup', {
      delegated_org_ids: ['org_existing', 'org_target'],
    }))
  })

  it('shares an OAuth subscription connected from organization settings', async () => {
    oauthStart.mockResolvedValue({
      data: { authorize_url: 'https://claude.ai/oauth', code_verifier: 'verifier', state: 'state' },
    })
    oauthComplete.mockResolvedValue({
      data: { id: 'csub_oauth', delegated_org_ids: ['org_existing'] },
    })
    delegationUpdate.mockResolvedValue({ data: {} })
    vi.spyOn(window, 'open').mockImplementation(() => null)

    renderConnect({ enableForOrgId: 'org_target' })
    fireEvent.click(await screen.findByRole('button', { name: /connect/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Claude' }))
    await waitFor(() => expect(oauthStart).toHaveBeenCalled())
    fireEvent.change(screen.getByLabelText('Authorization code'), {
      target: { value: 'oauth-code' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await waitFor(() => expect(delegationUpdate).toHaveBeenCalledWith('csub_oauth', {
      delegated_org_ids: ['org_existing', 'org_target'],
    }))
  })

  it('keeps OAuth dialog open when connection succeeds but sharing fails', async () => {
    oauthStart.mockResolvedValue({
      data: { authorize_url: 'https://claude.ai/oauth', code_verifier: 'verifier', state: 'state' },
    })
    oauthComplete.mockResolvedValue({ data: { id: 'csub_oauth' } })
    delegationUpdate.mockRejectedValue({
      response: { data: { message: 'organization uses API-provider mode' } },
    })
    vi.spyOn(window, 'open').mockImplementation(() => null)

    const onConnected = vi.fn()
    const { queryClient } = renderConnect({ enableForOrgId: 'org_target', onConnected })
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    fireEvent.click(await screen.findByRole('button', { name: /connect/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Claude' }))
    await waitFor(() => expect(oauthStart).toHaveBeenCalled())
    fireEvent.change(screen.getByLabelText('Authorization code'), { target: { value: 'oauth-code' } })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await screen.findByText(
      'Connected, but not shared with Acme: organization uses API-provider mode',
    )
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['claude-subscriptions'] })
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: codeAgentHarnessesQueryKey('org_target'),
    })
    expect(snackbarSuccess).not.toHaveBeenCalled()
    expect(onConnected).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^Connect$/ })).toBeInTheDocument()
  })

  it('keeps setup-token dialog open when connection succeeds but sharing fails', async () => {
    createSubscription.mockResolvedValue({ data: { id: 'csub_setup' } })
    delegationUpdate.mockRejectedValue({ response: { data: { message: 'sharing denied' } } })

    const onConnected = vi.fn()
    const { queryClient } = renderConnect({ enableForOrgId: 'org_target', onConnected })
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    await openDialogOnSetupToken()
    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 'p'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await screen.findByText('Connected, but not shared with Acme: sharing denied')
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['claude-subscriptions'] })
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: codeAgentHarnessesQueryKey('org_target'),
    })
    expect(snackbarSuccess).not.toHaveBeenCalled()
    expect(onConnected).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^Connect$/ })).toBeInTheDocument()
  })

  it('does not delegate without an organization target', async () => {
    createSubscription.mockResolvedValue({ data: { id: 'csub_setup' } })

    renderConnect()
    await openDialogOnSetupToken()
    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 'n'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await waitFor(() => expect(snackbarSuccess).toHaveBeenCalledWith('Claude subscription connected'))
    expect(delegationUpdate).not.toHaveBeenCalled()
  })

  it('does not delegate without a returned subscription ID', async () => {
    createSubscription.mockResolvedValue({ data: {} })

    renderConnect({ enableForOrgId: 'org_target' })
    await openDialogOnSetupToken()
    fireEvent.change(screen.getByLabelText(/Claude Code setup token/i), {
      target: { value: 'sk-ant-oat01-' + 'm'.repeat(60) },
    })
    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }))

    await screen.findByText(
      'Connected, but not shared with Acme: The connected subscription did not return an ID',
    )
    expect(delegationUpdate).not.toHaveBeenCalled()
    expect(snackbarSuccess).not.toHaveBeenCalled()
  })
})
