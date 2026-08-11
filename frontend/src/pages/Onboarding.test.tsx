import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Onboarding from './Onboarding'

const mockNavigateReplace = vi.fn()
const mockSnackbarError = vi.fn()
const mockLoadOrganizations = vi.fn()
const mockV1UsersMeOnboardingCreate = vi.fn()
const mockCreateOrgMutateAsync = vi.fn()

const mockState = vi.hoisted(() => ({
  walletStatus: 'active',
  claudeSubscriptions: [{ id: 'claude-sub-1' }] as Array<{ id: string }>,
  codexSubscriptions: [] as Array<{ id: string }>,
}))

let mockAccountValue: any

function setAccountWithOrgs(orgs: Array<{ id: string; name: string; display_name: string }>) {
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
    data: { billing_enabled: true, edition: 'cloud' },
    isLoading: false,
  }),
}))

vi.mock('../services/useBilling', () => ({
  useGetWallet: () => ({
    data: {
      subscription_status: mockState.walletStatus,
      subscription_created: 0,
      subscription_current_period_start: 0,
      subscription_current_period_end: 0,
      balance: 0,
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
    mockState.claudeSubscriptions = [{ id: 'claude-sub-1' }]
    mockState.codexSubscriptions = []
    mockV1UsersMeOnboardingCreate.mockResolvedValue({})
    setAccountWithOrgs([
      { id: 'org-1', name: 'my-org', display_name: 'My Org' },
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

    expect(
      screen.getByRole('button', { name: /continue with helix credits/i }),
    ).toBeEnabled()
    expect(screen.getByRole('button', { name: /helix providers/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /claude subscription/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chatgpt subscription/i })).toBeInTheDocument()
    expect(screen.queryByText(/create your first project/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/create your first task/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/where is your code/i)).not.toBeInTheDocument()
  })

  it('completes onboarding with Helix credits and opens org chat', async () => {
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
    expect(localStorage.getItem('selected_org')).toBe('my-org')
    expect(mockNavigateReplace).toHaveBeenCalledWith('org_chat', {
      org_id: 'my-org',
    })
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

  it('can finish with a connected external coding subscription', async () => {
    mockState.codexSubscriptions = [{ id: 'codex-sub-1' }]
    renderOnboarding()
    await goToCodingAccessStep()

    fireEvent.click(screen.getByRole('button', { name: /chatgpt subscription/i }))
    const continueButton = screen.getByRole('button', {
      name: /continue with chatgpt subscription/i,
    })
    expect(continueButton).toBeEnabled()

    await act(async () => {
      fireEvent.click(continueButton)
    })

    await waitFor(() => {
      expect(mockNavigateReplace).toHaveBeenCalledWith('org_chat', {
        org_id: 'my-org',
      })
    })
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
