import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Onboarding from './Onboarding'

const mockNavigateReplace = vi.fn()
const mockNavigate = vi.fn()
const mockSnackbarError = vi.fn()
const mockLoadOrganizations = vi.fn()
const mockV1UsersMeOnboardingCreate = vi.fn()
const mockV1OrgsSettingsUpdate = vi.fn()
const mockV1SubscriptionNewCreate = vi.fn()
const mockV1TopUpsNewCreate = vi.fn()
const mockRefetchWallet = vi.fn()
const mockUseGetWallet = vi.fn()
const mockCreateOrgMutateAsync = vi.fn()
const mockUpdateHarnesses = vi.fn()
const onboardingDraftKey = 'helix_onboarding_draft:v1:user-1'

const mockState = vi.hoisted(() => ({
  edition: 'cloud',
  billingEnabled: true,
  walletStatus: 'active',
  walletBalance: 42.5,
  providers: [] as any[],
  harnesses: [] as any[],
  onboardingHelixDefault: {
    provider: 'pe_helix',
    model: 'helix-model',
    effort: 'high',
  },
}))

let mockAccountValue: any

function setAccountWithOrgs(orgs: Array<{
  id: string
  name: string
  display_name: string
  owner?: string
  memberships?: Array<{ user_id: string; role: string }>
}>) {
  mockAccountValue = {
    user: { id: 'user-1', name: 'Test User', email: 'test@example.com' },
    organizationTools: {
      organizations: orgs,
      organization: orgs[0],
      loading: false,
      orgID: '',
      loadOrganizations: mockLoadOrganizations,
    },
    dismissOnboarding: vi.fn(),
    orgNavigate: vi.fn(),
  }
}

vi.mock('../hooks/useAccount', () => ({
  default: () => mockAccountValue,
}))

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    get: vi.fn(),
    getApiClient: () => ({
      v1UsersMeOnboardingCreate: mockV1UsersMeOnboardingCreate,
      v1OrgsSettingsUpdate: mockV1OrgsSettingsUpdate,
      v1SubscriptionNewCreate: mockV1SubscriptionNewCreate,
      v1TopUpsNewCreate: mockV1TopUpsNewCreate,
    }),
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({
    error: mockSnackbarError,
    success: vi.fn(),
    info: vi.fn(),
  }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    name: 'onboarding',
    params: {},
    meta: {},
    navigate: mockNavigate,
    navigateReplace: mockNavigateReplace,
    setParams: vi.fn(),
    mergeParams: vi.fn(),
    replaceParams: vi.fn(),
    removeParams: vi.fn(),
  }),
}))

vi.mock('../services/orgService', () => ({
  useCreateOrg: () => ({
    mutateAsync: mockCreateOrgMutateAsync,
    isPending: false,
  }),
}))

vi.mock('../services/userService', () => ({
  useGetConfig: () => ({
    data: {
      billing_enabled: mockState.billingEnabled,
      edition: mockState.edition,
      onboarding_helix_model_provider: mockState.onboardingHelixDefault.provider,
      onboarding_helix_model: mockState.onboardingHelixDefault.model,
      onboarding_helix_model_effort: mockState.onboardingHelixDefault.effort,
    },
    isLoading: false,
  }),
}))

vi.mock('../services/codeAgentHarnessesService', () => ({
  useUpdateOrgCodeAgentHarnesses: () => ({ mutateAsync: mockUpdateHarnesses }),
  useOrgCodeAgentHarnesses: () => ({
    data: mockState.harnesses,
    isLoading: false,
  }),
  findHarnessStatus: (harnesses: any[], runtime: string) =>
    harnesses.find((harness) => harness.runtime === runtime),
}))

vi.mock('../services/providersService', () => ({
  useListProviders: () => ({ data: mockState.providers, isLoading: false }),
}))

vi.mock('../services/useBilling', () => ({
  TOP_UP_AMOUNTS: [5, 10, 20, 50, 100],
  DEFAULT_TOP_UP_AMOUNT: 5,
  useGetWallet: (...args: unknown[]) => {
    mockUseGetWallet(...args)
    return {
      data: {
        subscription_status: mockState.walletStatus,
        subscription_created: 0,
        subscription_current_period_start: 0,
        subscription_current_period_end: 0,
        balance: mockState.walletBalance,
      },
      refetch: mockRefetchWallet,
      isFetching: false,
    }
  },
}))

vi.mock('../components/account/ClaudeSubscriptionConnect', () => ({
  default: ({ enableForOrgId }: { enableForOrgId?: string }) => (
    <button data-enable-for-org-id={enableForOrgId}>Connect Claude</button>
  ),
}))

vi.mock('../components/account/CodexSubscriptionConnect', () => ({
  default: ({ enableForOrgId }: { enableForOrgId?: string }) => (
    <button data-enable-for-org-id={enableForOrgId}>Connect ChatGPT</button>
  ),
}))

vi.mock('lucide-react', () => ({
  Bot: () => <span data-testid="bot-icon" />,
  Server: () => <span data-testid="server-icon" />,
}))

function renderOnboarding() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <Onboarding />
    </QueryClientProvider>,
  )
}

async function goToCodingAccessStep() {
  fireEvent.click(
    screen.getByRole('button', { name: /continue with this organization/i }),
  )

  if (mockState.billingEnabled) {
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^continue$/i })).toBeInTheDocument()
    })
    fireEvent.click(screen.getByRole('button', { name: /^continue$/i }))
  }

  await waitFor(() => {
    expect(screen.getByText('Choose how to run coding agents')).toBeInTheDocument()
  })
}

describe('Onboarding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    window.history.replaceState({}, '', '/onboarding')
    mockState.edition = 'cloud'
    mockState.billingEnabled = true
    mockState.walletStatus = 'active'
    mockState.walletBalance = 42.5
    mockState.onboardingHelixDefault = {
      provider: 'pe_helix',
      model: 'helix-model',
      effort: 'high',
    }
    mockState.providers = [{
      id: 'pe_helix',
      name: 'helix',
      status: 'ok',
      available_models: [{ id: 'helix-model', enabled: true, type: 'chat' }],
    }]
    mockState.harnesses = [
      { runtime: 'zed_agent', enabled: true, subscription_enabled: false },
      { runtime: 'claude_code', enabled: false, subscription_enabled: false, viewer_has_subscription: true },
      { runtime: 'codex_cli', enabled: false, subscription_enabled: false, viewer_has_subscription: false },
    ]
    mockV1UsersMeOnboardingCreate.mockResolvedValue({})
    mockV1OrgsSettingsUpdate.mockResolvedValue({})
    mockV1SubscriptionNewCreate.mockResolvedValue({})
    mockV1TopUpsNewCreate.mockResolvedValue({})
    mockCreateOrgMutateAsync.mockResolvedValue(undefined)
    mockUpdateHarnesses.mockResolvedValue([])
    setAccountWithOrgs([
      { id: 'org-1', name: 'my-org', display_name: 'My Org', owner: 'user-1' },
    ])
  })

  it('greets the user by their full name', () => {
    renderOnboarding()

    expect(screen.getByText('Hello, Test User')).toBeInTheDocument()
    expect(screen.getByText("Let's set you up for success 😉")).toBeInTheDocument()
  })

  it('ends after coding access and defaults to Helix credits', async () => {
    renderOnboarding()
    await goToCodingAccessStep()

    expect(screen.getByRole('button', { name: 'Meet your Chief of Staff' })).toBeEnabled()
    expect(screen.getByRole('button', { name: /helix providers/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /claude subscription/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chatgpt subscription/i })).toBeInTheDocument()
    expect(screen.getByText(/You have 42.50 Helix credits/)).toBeInTheDocument()
    expect(screen.getByText(/Claude Code or Codex to use your own subscription/)).toBeInTheDocument()
    expect(screen.getByText(/those runs do not use Helix credits/)).toBeInTheDocument()
    expect(screen.getByText('Recommended model: helix-model')).toBeInTheDocument()
    expect(screen.queryByLabelText('Helix provider')).not.toBeInTheDocument()
    expect(screen.queryByText(/create your first project/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/create your first task/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/where is your code/i)).not.toBeInTheDocument()
  })

  it('shows provider cards as selectable actions without a dismiss control', async () => {
    renderOnboarding()

    expect(screen.queryByTestId('CloseIcon')).not.toBeInTheDocument()
    await goToCodingAccessStep()

    const helix = screen.getByRole('button', { name: /helix providers/i })
    const claude = screen.getByRole('button', { name: /claude subscription/i })
    const codex = screen.getByRole('button', { name: /chatgpt subscription/i })

    expect(helix).toHaveAttribute('aria-pressed', 'true')
    expect(claude).toHaveAttribute('aria-pressed', 'false')
    expect(codex).toHaveAttribute('aria-pressed', 'false')
    expect(within(helix).getByText('Selected')).toBeVisible()
    expect(within(claude).getByText('Select')).toBeVisible()
    expect(within(codex).getByText('Select')).toBeVisible()

    fireEvent.click(claude)

    expect(helix).toHaveAttribute('aria-pressed', 'false')
    expect(claude).toHaveAttribute('aria-pressed', 'true')
    expect(within(helix).getByText('Select')).toBeVisible()
    expect(within(claude).getByText('Selected')).toBeVisible()
  })

  it('saves the selected Helix runtime and opens the Chief of Staff on cloud', async () => {
    renderOnboarding()
    await goToCodingAccessStep()

    await waitFor(() => expect(localStorage.getItem(onboardingDraftKey)).not.toBeNull())

    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Meet your Chief of Staff' }),
      )
    })

    await waitFor(() => {
      expect(mockV1UsersMeOnboardingCreate).toHaveBeenCalledTimes(1)
    })
    expect(mockAccountValue.dismissOnboarding).toHaveBeenCalledTimes(1)
    expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith('agent.default', 'my-org', {
      value: JSON.stringify({
        code_agent_runtime: 'zed_agent',
        code_agent_credential_type: 'api_key',
        provider: 'pe_helix',
        model: 'helix-model',
        reasoning_effort: 'high',
      }),
    })
    expect(localStorage.getItem('selected_org')).toBe('my-org')
    expect(localStorage.getItem(onboardingDraftKey)).toBeNull()
    expect(mockNavigateReplace).toHaveBeenCalledWith('org_bot_session', {
      org_id: 'my-org',
      bot_id: 'chief-of-staff',
    })
    expect(mockUpdateHarnesses).not.toHaveBeenCalled()
  })

  it('restores onboarding progress and choices after remounting', async () => {
    const firstRender = renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Claude model'))
    fireEvent.click(screen.getByRole('option', { name: /Claude Fable 5/i }))

    await waitFor(() => {
      expect(JSON.parse(localStorage.getItem(onboardingDraftKey) || '{}')).toMatchObject({
        activeStepType: 'provider',
        completedStepTypes: ['signin', 'organization', 'subscription'],
        createdOrgId: 'org-1',
        codingAccessOption: 'claude',
        claudeModel: 'claude-fable-5',
      })
    })
    firstRender.unmount()

    renderOnboarding()

    await screen.findByText('Choose how to run coding agents')
    expect(screen.getByRole('button', { name: /claude subscription/i })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByLabelText('Claude model')).toHaveTextContent('Claude Fable 5')
  })

  it('requires a Claude connection only when Claude is selected', async () => {
    mockState.harnesses = mockState.harnesses.map((harness) =>
      harness.runtime === 'claude_code'
        ? { ...harness, viewer_has_subscription: false }
        : harness)
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))

    expect(screen.getByRole('button', { name: 'Connect Claude' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Meet your Chief of Staff' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Connect Claude' })).toHaveAttribute(
      'data-enable-for-org-id',
      'org-1',
    )
  })

  it('requires a ChatGPT connection only when ChatGPT is selected', async () => {
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))

    expect(screen.getByRole('button', { name: 'Connect ChatGPT' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Meet your Chief of Staff' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Connect ChatGPT' })).toHaveAttribute(
      'data-enable-for-org-id',
      'org-1',
    )
  })

  it('selects a Codex subscription model and enables only that harness', async () => {
    mockState.harnesses = mockState.harnesses.map((harness) =>
      harness.runtime === 'codex_cli'
        ? { ...harness, viewer_has_subscription: true }
        : harness)
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Codex model'))
    fireEvent.click(screen.getByRole('option', { name: 'GPT-5.6 Terra' }))
    const continueButton = screen.getByRole('button', {
      name: 'Meet your Chief of Staff',
    })
    expect(continueButton).toBeEnabled()

    await act(async () => {
      fireEvent.click(continueButton)
    })

    await waitFor(() => {
      expect(mockNavigateReplace).toHaveBeenCalledWith('org_bot_session', {
        org_id: 'my-org',
        bot_id: 'chief-of-staff',
      })
    })
    expect(mockUpdateHarnesses).toHaveBeenCalledWith([{
      runtime: 'codex_cli',
      enabled: true,
      subscription_enabled: true,
    }])
    expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith('agent.default', 'my-org', {
      value: JSON.stringify({
        code_agent_runtime: 'codex_cli',
        code_agent_credential_type: 'subscription',
        provider: '',
        model: 'gpt-5.6-terra',
        reasoning_effort: 'none',
      }),
    })
  })

  it('keeps project creation for an existing self-hosted organization', async () => {
    mockState.edition = 'self-hosted'
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Claude model'))
    fireEvent.click(screen.getByRole('option', { name: /Claude Fable 5/i }))
    fireEvent.click(screen.getByRole('button', { name: /continue with claude subscription/i }))

    await waitFor(() => expect(mockNavigateReplace).toHaveBeenCalledWith('org_projects', {
      org_id: 'my-org',
      create_project_config: JSON.stringify({
        runtime: 'claude_code',
        credential_type: 'subscription',
        model: 'claude-fable-5',
      }),
    }))
    expect(mockUpdateHarnesses).toHaveBeenCalledWith([{
      runtime: 'claude_code',
      enabled: true,
      subscription_enabled: true,
    }])
    expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith('agent.default', 'my-org', {
      value: JSON.stringify({
        code_agent_runtime: 'claude_code',
        code_agent_credential_type: 'subscription',
        provider: '',
        model: 'claude-fable-5',
        reasoning_effort: 'none',
      }),
    })
  })

  it('blocks Helix completion when no allowed provider is available', async () => {
    mockState.providers = []
    renderOnboarding()
    await goToCodingAccessStep()

    expect(screen.queryByRole('button', { name: 'Meet your Chief of Staff' })).not.toBeInTheDocument()
  })

  it('offers credits and provider connections instead of Chief of Staff when balance is zero', async () => {
    mockState.walletBalance = 0
    mockState.harnesses = mockState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: false,
    }))
    renderOnboarding()
    await goToCodingAccessStep()

    expect(screen.queryByRole('button', { name: 'Meet your Chief of Staff' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add credits' })).toBeEnabled()
    expect(screen.getByRole('combobox', { name: 'Top-up amount' })).toHaveTextContent('$5')
    expect(screen.getByRole('button', { name: /claude subscription/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chatgpt subscription/i })).toBeInTheDocument()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Add credits' }))
    })
    expect(mockV1TopUpsNewCreate).toHaveBeenCalledWith({
      amount: 5,
      org_id: 'org-1',
      return_url: '/onboarding?org_id=org-1&step=provider',
    })
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('preserves newly-created organization context through top-up checkout', async () => {
    mockState.walletBalance = 0
    setAccountWithOrgs([])
    mockCreateOrgMutateAsync.mockResolvedValue({
      id: 'org-2',
      name: 'new-org',
      display_name: 'New Org',
    })
    renderOnboarding()

    fireEvent.change(screen.getByLabelText('Organization name'), {
      target: { value: 'New Org' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create organization' }))
    await waitFor(() => expect(screen.getByRole('button', { name: /^continue$/i })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: /^continue$/i }))
    fireEvent.mouseDown(await screen.findByRole('combobox', { name: 'Top-up amount' }))
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual([
      '$5',
      '$10',
      '$20',
      '$50',
      '$100',
    ])
    fireEvent.click(screen.getByRole('option', { name: '$100' }))
    await act(async () => {
      fireEvent.click(await screen.findByRole('button', { name: 'Add credits' }))
    })

    expect(mockV1TopUpsNewCreate).toHaveBeenCalledWith({
      amount: 100,
      org_id: 'org-2',
      return_url: '/onboarding?org_id=org-2&step=provider&created_org=true',
    })
  })

  it('defaults a connected Claude model so onboarding can finish', async () => {
    mockState.walletBalance = 0
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))
    const button = await screen.findByRole('button', { name: 'Meet your Chief of Staff' })
    fireEvent.click(button)

    await waitFor(() => expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith(
      'agent.default',
      'my-org',
      { value: JSON.stringify({
        code_agent_runtime: 'claude_code',
        code_agent_credential_type: 'subscription',
        provider: '',
        model: 'claude-opus-5',
        reasoning_effort: 'none',
      }) },
    ))
  })

  it('defaults a connected ChatGPT model so onboarding can finish', async () => {
    mockState.harnesses = mockState.harnesses.map((harness) =>
      harness.runtime === 'codex_cli'
        ? { ...harness, viewer_has_subscription: true }
        : harness)
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))
    const button = await screen.findByRole('button', { name: 'Meet your Chief of Staff' })
    fireEvent.click(button)

    await waitFor(() => expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith(
      'agent.default',
      'my-org',
      { value: JSON.stringify({
        code_agent_runtime: 'codex_cli',
        code_agent_credential_type: 'subscription',
        provider: '',
        model: 'gpt-5.6-sol',
        reasoning_effort: 'none',
      }) },
    ))
  })

  it('restores the provider step after a successful top-up', async () => {
    window.history.replaceState(
      {},
      '',
      '/onboarding?org_id=org-1&step=provider&created_org=true&success=true&session_id=cs_1',
    )
    renderOnboarding()

    await screen.findByText('Choose how to run coding agents')
    expect(screen.getByRole('button', { name: 'Meet your Chief of Staff' })).toBeEnabled()
    expect(mockRefetchWallet).toHaveBeenCalled()
    expect(window.location.search).toBe('?org_id=org-1&step=provider&created_org=true')
  })

  it('shows confirmation and refreshes the wallet after returning from subscription checkout', async () => {
    mockState.walletStatus = 'not_subscribed'
    setAccountWithOrgs([
      { id: 'org-1', name: 'my-org', display_name: 'My Org', owner: 'user-1' },
    ])
    window.history.replaceState(
      {},
      '',
      '/onboarding?org_id=org-1&success=true&session_id=cs_1',
    )

    renderOnboarding()

    expect(await screen.findByText('Confirming your free trial with Stripe...')).toBeInTheDocument()
    expect(mockUseGetWallet).toHaveBeenCalledWith('org-1', true, true)
    await waitFor(() => expect(mockRefetchWallet).toHaveBeenCalled())
  })

  it('requires a non-owner to ask the owner before changing subscription policy', async () => {
    setAccountWithOrgs([
      { id: 'org-1', name: 'my-org', display_name: 'My Org', owner: 'another-user' },
    ])
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Claude model'))
    fireEvent.click(screen.getByRole('option', { name: /Claude Fable 5/i }))

    expect(screen.getByText(
      'Ask an organization owner to set the Default Runtime.',
    )).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Meet your Chief of Staff' })).toBeDisabled()
    expect(mockUpdateHarnesses).not.toHaveBeenCalled()
  })

  it('does not rewrite policy when subscription access is already enabled', async () => {
    mockState.harnesses = mockState.harnesses.map((harness) =>
      harness.runtime === 'claude_code'
        ? { ...harness, enabled: true, subscription_enabled: true }
        : harness)
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Claude model'))
    fireEvent.click(screen.getByRole('option', { name: /Claude Fable 5/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Meet your Chief of Staff' }))

    await waitFor(() => expect(mockNavigateReplace).toHaveBeenCalledWith(
      'org_bot_session',
      { org_id: 'my-org', bot_id: 'chief-of-staff' },
    ))
    expect(mockUpdateHarnesses).not.toHaveBeenCalled()
  })

  it('opens the Chief of Staff for an organization created during self-hosted onboarding', async () => {
    mockState.edition = 'self-hosted'
    mockState.billingEnabled = false
    setAccountWithOrgs([])
    mockCreateOrgMutateAsync.mockResolvedValue({
      id: 'org-2',
      name: 'new-org',
      display_name: 'New Org',
    })
    renderOnboarding()

    fireEvent.change(screen.getByLabelText('Organization name'), {
      target: { value: 'New Org' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create organization' }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Meet your Chief of Staff' })).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Meet your Chief of Staff' }))

    await waitFor(() => expect(mockNavigateReplace).toHaveBeenCalledWith(
      'org_bot_session',
      { org_id: 'new-org', bot_id: 'chief-of-staff' },
    ))
  })

  it('restores a new self-hosted organization after Stripe returns', async () => {
    mockState.edition = 'self-hosted'
    mockState.walletStatus = 'not_subscribed'
    setAccountWithOrgs([])
    mockCreateOrgMutateAsync.mockResolvedValue({
      id: 'org-2',
      name: 'new-org',
      display_name: 'New Org',
    })
    const firstRender = renderOnboarding()

    fireEvent.change(screen.getByLabelText('Organization name'), {
      target: { value: 'New Org' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create organization' }))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /start 72-hour free trial/i })).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: /start 72-hour free trial/i }))

    await waitFor(() => expect(mockV1SubscriptionNewCreate).toHaveBeenCalledWith({
      org_id: 'org-2',
      return_url: '/onboarding?org_id=org-2&created_org=true',
    }))
    firstRender.unmount()

    mockState.walletStatus = 'active'
    setAccountWithOrgs([
      { id: 'org-2', name: 'new-org', display_name: 'New Org', owner: 'user-1' },
    ])
    window.history.replaceState(
      {},
      '',
      '/onboarding?org_id=org-2&created_org=true&success=true',
    )
    renderOnboarding()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^continue$/i })).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: /^continue$/i }))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Meet your Chief of Staff' })).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Meet your Chief of Staff' }))

    await waitFor(() => expect(mockNavigateReplace).toHaveBeenCalledWith(
      'org_bot_session',
      { org_id: 'new-org', bot_id: 'chief-of-staff' },
    ))
  })

  it('shows benefits rather than empty billing fields before subscription', async () => {
    mockState.walletStatus = 'not_subscribed'
    renderOnboarding()

    fireEvent.click(
      screen.getByRole('button', { name: /continue with this organization/i }),
    )

    await waitFor(() => {
      expect(
        screen.getByText(/full linux desktop sandboxes/i),
      ).toBeInTheDocument()
    })
    expect(screen.queryByText(/status: not_subscribed/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/current balance:/i)).not.toBeInTheDocument()
    expect(screen.getByText(/card will not be charged for 72 hours/i)).toBeInTheDocument()
    expect(screen.getByText(/automatically continues for \$499\/month/i)).toBeInTheDocument()
    expect(screen.getByText(/cancel before the trial ends/i)).toBeInTheDocument()
  })
})
