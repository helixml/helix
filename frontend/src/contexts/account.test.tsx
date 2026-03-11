import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'

// Mutable state for mock factories (vi.mock is hoisted above imports)
const mockRouterState = { name: 'home' }
const mockOrgState = {
  initialized: true,
  organizations: [] as any[],
}

const mockNavigateReplace = vi.fn()
const mockNavigate = vi.fn()
const mockLoadOrganizations = vi.fn().mockResolvedValue(undefined)
const mockApiGet = vi.fn()
const mockV1AuthAuthenticatedList = vi.fn()
const mockV1AuthUserList = vi.fn()

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    get: mockApiGet,
    post: vi.fn(),
    getApiClient: () => ({
      v1AuthAuthenticatedList: mockV1AuthAuthenticatedList,
      v1AuthUserList: mockV1AuthUserList,
    }),
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn(),
  }),
}))

vi.mock('../hooks/useLoading', () => ({
  default: () => ({
    setLoading: vi.fn(),
  }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    name: mockRouterState.name,
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

vi.mock('../hooks/useOrganizations', () => ({
  default: () => ({
    initialized: mockOrgState.initialized,
    organizations: mockOrgState.organizations,
    loading: false,
    orgID: '',
    organization: undefined,
    loadOrganizations: mockLoadOrganizations,
    createOrganization: vi.fn(),
    updateOrganization: vi.fn(),
    deleteOrganization: vi.fn(),
    loadOrganization: vi.fn(),
    addMemberToOrganization: vi.fn(),
    deleteMemberFromOrganization: vi.fn(),
    updateOrganizationMemberRole: vi.fn(),
    createTeam: vi.fn(),
    createTeamWithCreator: vi.fn(),
    updateTeam: vi.fn(),
    deleteTeam: vi.fn(),
    addTeamMember: vi.fn(),
    removeTeamMember: vi.fn(),
    searchUsers: vi.fn(),
    listAppAccessGrants: vi.fn(),
    createAppAccessGrant: vi.fn(),
    updateAppAccessGrant: vi.fn(),
    deleteAppAccessGrant: vi.fn(),
    appAccessGrants: [],
    loadingAccessGrants: false,
  }),
  defaultOrganizationTools: {
    organizations: [],
    loading: false,
    initialized: false,
    orgID: '',
  },
}))

import { useAccountContext } from './account'

function setupAuthenticatedUser(overrides: Record<string, any> = {}) {
  mockV1AuthAuthenticatedList.mockResolvedValue({ data: { authenticated: true } })
  mockV1AuthUserList.mockResolvedValue({
    data: {
      id: 'user-1',
      name: 'Test User',
      email: 'test@example.com',
      waitlisted: false,
      onboarding_completed: false,
      ...overrides,
    },
  })
}

function setupUnauthenticated() {
  mockV1AuthAuthenticatedList.mockResolvedValue({ data: { authenticated: false } })
}

function setupDefaultApiResponses() {
  mockApiGet.mockImplementation((url: string) => {
    if (url === '/api/v1/status') {
      return Promise.resolve({ credits: 0, admin: false, config: {} })
    }
    if (url === '/api/v1/config') {
      return Promise.resolve({
        filestore_prefix: '',
        stripe_enabled: false,
        sentry_dsn_frontend: '',
        google_analytics_frontend: '',
        eval_user_id: '',
        tools_enabled: true,
        apps_enabled: true,
      })
    }
    return Promise.resolve(null)
  })
}

describe('useAccountContext redirect logic', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockRouterState.name = 'home'
    mockOrgState.initialized = true
    mockOrgState.organizations = []
    setupDefaultApiResponses()
  })

  describe('redirect loop prevention', () => {
    it('fresh user with no orgs and incomplete onboarding redirects only to /onboarding, never /orgs', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('onboarding')
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })
  })

  describe('onboarding redirect', () => {
    it('redirects to /onboarding when onboarding not completed and no orgs', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('onboarding')
    })

    it('does not redirect to /onboarding when already on onboarding page', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })
      mockRouterState.name = 'onboarding'

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
    })

    it('does not redirect to /onboarding when onboarding is completed', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
    })

    it('does not redirect to /onboarding when user has organizations', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })
      mockOrgState.organizations = [{ id: 'org-1', name: 'my-org', member: true }]

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
    })

    it('does not redirect to /onboarding when user is waitlisted', async () => {
      setupAuthenticatedUser({ waitlisted: true, onboarding_completed: false })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('waitlist')
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
    })
  })

  describe('orgs redirect', () => {
    it('redirects to /orgs when onboarding completed and no org memberships', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('orgs')
    })

    it('does not redirect to /orgs when onboarding is not completed (prevents loop)', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })

    it('does not redirect to /orgs when already on orgs page', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })
      mockRouterState.name = 'orgs'

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })

    it('does not redirect to /orgs when user has org memberships', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })
      mockOrgState.organizations = [{ id: 'org-1', name: 'my-org', member: true }]

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })

    it('redirects to /orgs when orgs exist but none have membership', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })
      mockOrgState.organizations = [
        { id: 'org-1', name: 'org-a', member: false },
        { id: 'org-2', name: 'org-b', member: false },
      ]

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('orgs')
    })
  })

  describe('onboarding dismiss', () => {
    it('after dismissing onboarding with no orgs, redirects to /orgs', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })

      const { result, rerender } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(mockNavigateReplace).toHaveBeenCalledWith('onboarding')
      })
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')

      // Dismiss onboarding and change route to trigger effect re-evaluation
      act(() => {
        result.current.dismissOnboarding()
      })
      mockNavigateReplace.mockClear()
      mockRouterState.name = 'some-page'
      rerender()

      await waitFor(() => {
        expect(mockNavigateReplace).toHaveBeenCalledWith('orgs')
      })
    })
  })

  describe('waitlist redirect', () => {
    it('redirects to /waitlist and blocks onboarding/orgs redirects', async () => {
      setupAuthenticatedUser({ waitlisted: true, onboarding_completed: false })

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('waitlist')
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })
  })

  describe('login redirect', () => {
    it('redirects to /login when not authenticated', async () => {
      setupUnauthenticated()

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('login')
    })
  })

  describe('waiting for org data', () => {
    it('does not redirect to /onboarding or /orgs until organizations are initialized', async () => {
      setupAuthenticatedUser({ onboarding_completed: false })
      mockOrgState.initialized = false

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalledWith('onboarding')
      expect(mockNavigateReplace).not.toHaveBeenCalledWith('orgs')
    })
  })

  describe('happy path - no redirects', () => {
    it('does not redirect when onboarding completed and has org memberships', async () => {
      setupAuthenticatedUser({ onboarding_completed: true })
      mockOrgState.organizations = [{ id: 'org-1', name: 'my-org', member: true }]

      const { result } = renderHook(() => useAccountContext())

      await waitFor(() => {
        expect(result.current.initialized).toBe(true)
      })

      expect(mockNavigateReplace).not.toHaveBeenCalled()
    })
  })
})
