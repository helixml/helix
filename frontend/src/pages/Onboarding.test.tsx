import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Onboarding from './Onboarding'

const mockNavigateReplace = vi.fn()
const mockSnackbarError = vi.fn()
const mockLoadOrganizations = vi.fn()
const mockV1UsersMeOnboardingCreate = vi.fn()
const mockV1OrgsSettingsUpdate = vi.fn()
const mockCreateOrgMutateAsync = vi.fn()
const mockUpdateHarnesses = vi.fn()

const mockState = vi.hoisted(() => ({
  walletStatus: 'active',
  claudeSubscriptions: [{ id: 'claude-sub-1' }] as Array<{ id: string }>,
  codexSubscriptions: [] as Array<{ id: string }>,
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
      v1SubscriptionNewCreate: vi.fn(),
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
    navigate: vi.fn(),
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
      billing_enabled: true,
      edition: 'cloud',
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
  useGetWallet: () => ({
    data: {
      subscription_status: mockState.walletStatus,
      subscription_created: 0,
      subscription_current_period_start: 0,
      subscription_current_period_end: 0,
      balance: 42.5,
    },
    refetch: vi.fn(),
    isFetching: false,
  }),
}))

vi.mock('../components/account/ClaudeSubscriptionConnect', () => ({
  default: () => <button>Connect Claude</button>,
  useClaudeSubscriptions: () => ({ data: mockState.claudeSubscriptions }),
}))

vi.mock('../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: mockState.codexSubscriptions }),
}))

vi.mock('../components/account/CodexSubscriptionConnect', () => ({
  default: () => <button>Connect ChatGPT</button>,
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

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /^continue$/i })).toBeInTheDocument()
  })
  fireEvent.click(screen.getByRole('button', { name: /^continue$/i }))

  await waitFor(() => {
    expect(screen.getByText('Choose how to run coding agents')).toBeInTheDocument()
  })
}

describe('Onboarding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockState.walletStatus = 'active'
    mockState.onboardingHelixDefault = {
      provider: 'pe_helix',
      model: 'helix-model',
      effort: 'high',
    }
    mockState.claudeSubscriptions = [{ id: 'claude-sub-1' }]
    mockState.codexSubscriptions = []
    mockState.providers = [{
      id: 'pe_helix',
      name: 'helix',
      status: 'ok',
      available_models: [{ id: 'helix-model', enabled: true, type: 'chat' }],
    }]
    mockState.harnesses = [
      { runtime: 'zed_agent', enabled: true, subscription_enabled: false },
      { runtime: 'claude_code', enabled: false, subscription_enabled: false },
      { runtime: 'codex_cli', enabled: false, subscription_enabled: false },
    ]
    mockV1UsersMeOnboardingCreate.mockResolvedValue({})
    mockV1OrgsSettingsUpdate.mockResolvedValue({})
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

    expect(screen.getByRole('button', { name: /continue with helix credits/i })).toBeEnabled()
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

  it('saves the selected Helix runtime and opens project creation', async () => {
    renderOnboarding()
    await goToCodingAccessStep()

    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: /continue with helix credits/i }),
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
    expect(mockNavigateReplace).toHaveBeenCalledWith('org_projects', {
      org_id: 'my-org',
      create_project_config: JSON.stringify({
        runtime: 'zed_agent',
        credential_type: 'api_key',
        provider_ref: 'pe_helix',
        model: 'helix-model',
        reasoning_effort: 'high',
      }),
    })
    expect(mockUpdateHarnesses).not.toHaveBeenCalled()
  })

  it('requires a Claude connection only when Claude is selected', async () => {
    mockState.claudeSubscriptions = []
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /claude subscription/i }))

    expect(screen.getByRole('button', { name: 'Connect Claude' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /continue with claude subscription/i }),
    ).toBeDisabled()
  })

  it('requires a ChatGPT connection only when ChatGPT is selected', async () => {
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))

    expect(screen.getByRole('button', { name: 'Connect ChatGPT' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /continue with chatgpt subscription/i }),
    ).toBeDisabled()
  })

  it('selects a Codex subscription model and enables only that harness', async () => {
    mockState.codexSubscriptions = [{ id: 'codex-sub-1' }]
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))
    fireEvent.mouseDown(screen.getByLabelText('Codex model'))
    fireEvent.click(screen.getByRole('option', { name: 'GPT-5.6 Terra' }))
    const continueButton = screen.getByRole('button', {
      name: /continue with chatgpt subscription/i,
    })
    expect(continueButton).toBeEnabled()

    await act(async () => {
      fireEvent.click(continueButton)
    })

    await waitFor(() => {
      expect(mockNavigateReplace).toHaveBeenCalledWith('org_projects', {
        org_id: 'my-org',
        create_project_config: JSON.stringify({
          runtime: 'codex_cli',
          credential_type: 'subscription',
          model: 'gpt-5.6-terra',
        }),
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

  it('selects a Claude subscription model and passes it to project creation', async () => {
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

    expect(screen.getByRole('button', { name: /continue with helix credits/i })).toBeDisabled()
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
    expect(screen.getByRole('button', { name: /continue with claude subscription/i })).toBeDisabled()
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
    fireEvent.click(screen.getByRole('button', { name: /continue with claude subscription/i }))

    await waitFor(() => expect(mockNavigateReplace).toHaveBeenCalledWith(
      'org_projects',
      expect.any(Object),
    ))
    expect(mockUpdateHarnesses).not.toHaveBeenCalled()
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
  })
})
