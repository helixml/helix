import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CodexSubscriptionConnect, { CODEX_DEVICE_AUTH_URL } from './CodexSubscriptionConnect'

const startLoginMutate = vi.fn()
const cancelLoginMutate = vi.fn()
const deleteMutate = vi.fn()
const createMutateAsync = vi.fn()
let subscriptions: { id: string; owner_type: 'user' | 'org'; owner_id: string }[] = []
let loginStatus: { found?: boolean; code?: string; url?: string; error?: string } | undefined

vi.mock('../../services/codexSubscriptionsService', () => ({
  codexSubscriptionsQueryKey: ['codex-subscriptions'],
  useCodexSubscriptions: () => ({ data: subscriptions }),
  useCreateCodexSubscription: () => ({ mutateAsync: createMutateAsync, isPending: false }),
  useDeleteCodexSubscription: () => ({ mutate: deleteMutate, isPending: false }),
  useStartCodexLogin: () => ({
    mutate: startLoginMutate,
    isPending: false,
  }),
  useCancelCodexLogin: () => ({ mutate: cancelLoginMutate, isPending: false }),
  usePollCodexLogin: () => ({ data: loginStatus }),
}))

function renderConnect(props: { orgId?: string; enableForOrgId?: string } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const component = () => (
    <QueryClientProvider client={client}>
      <CodexSubscriptionConnect {...props} />
    </QueryClientProvider>
  )
  const result = render(component())
  return { ...result, rerenderConnect: () => result.rerender(component()) }
}

describe('CodexSubscriptionConnect', () => {
  beforeEach(() => {
    subscriptions = []
    loginStatus = undefined
    startLoginMutate.mockReset()
    cancelLoginMutate.mockReset()
    deleteMutate.mockReset()
    createMutateAsync.mockReset()
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
    expect(startLoginMutate).toHaveBeenCalledWith(
      { organization_id: undefined },
      expect.any(Object),
    )
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
    expect(screen.getByText('Waiting for OpenAI callback…')).toBeInTheDocument()
    expect(screen.getByText(/Keep this dialog open/)).toBeInTheDocument()
  })

  it('shows completion feedback after the OpenAI callback', async () => {
    startLoginMutate.mockImplementation((_value, options) => options.onSuccess({ session_id: 'ses_login' }))
    const view = renderConnect({ enableForOrgId: 'org_1' })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Generate code' }))

    loginStatus = { found: true }
    view.rerenderConnect()

    await waitFor(() => expect(screen.getByText('Codex connected')).toBeInTheDocument())
    expect(screen.getByRole('dialog', { name: 'ChatGPT Subscription Connected' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Done' })).toBeInTheDocument()
    expect(startLoginMutate).toHaveBeenCalledWith(
      { organization_id: 'org_1' },
      expect.any(Object),
    )
  })

  it('replaces callback progress with an error', () => {
    loginStatus = {
      found: false,
      code: '2E5J-JKA6Q',
      url: CODEX_DEVICE_AUTH_URL,
      error: 'OpenAI rejected the authorization',
    }
    startLoginMutate.mockImplementation((_value, options) => options.onSuccess({ session_id: 'ses_login' }))
    renderConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Generate code' }))

    expect(screen.getByRole('alert')).toHaveTextContent('OpenAI rejected the authorization')
    expect(screen.queryByText('Waiting for OpenAI callback…')).not.toBeInTheDocument()
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

  it('targets harness enablement without changing subscription ownership', async () => {
    createMutateAsync.mockResolvedValue({ id: 'sub_1' })
    renderConnect({ enableForOrgId: 'org_1' })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    fireEvent.click(screen.getByRole('button', { name: 'Enter credentials' }))

    const credentials = {
      auth_mode: 'chatgpt',
      last_refresh: '2026-08-17T00:00:00Z',
      tokens: {
        id_token: 'id',
        access_token: 'access',
        refresh_token: 'refresh',
        account_id: 'account',
      },
    }
    fireEvent.change(screen.getByLabelText('Codex auth.json'), {
      target: { value: JSON.stringify(credentials) },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Import' }))

    await waitFor(() => expect(createMutateAsync).toHaveBeenCalledWith({
      name: 'My Codex Subscription',
      credentials,
      organization_id: 'org_1',
    }))
  })
})
