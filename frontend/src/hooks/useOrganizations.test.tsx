import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'

// vi.mock is hoisted above imports, so mock state lives in mutable objects.
const mockRouterState = { name: 'org_chat', params: {} as Record<string, string> }
const mockNavigate = vi.fn()
const mockSnackbarError = vi.fn()

const mockV1OrganizationsList = vi.fn()
const mockV1OrganizationsDetail = vi.fn()
const mockV1OrganizationsMembersDetail = vi.fn()
const mockV1OrganizationsRolesDetail = vi.fn()
const mockV1OrganizationsTeamsDetail = vi.fn()

vi.mock('./useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1OrganizationsList: mockV1OrganizationsList,
      v1OrganizationsDetail: mockV1OrganizationsDetail,
      v1OrganizationsMembersDetail: mockV1OrganizationsMembersDetail,
      v1OrganizationsRolesDetail: mockV1OrganizationsRolesDetail,
      v1OrganizationsTeamsDetail: mockV1OrganizationsTeamsDetail,
      v1OrganizationsTeamsMembersDetail: vi.fn(),
    }),
  }),
}))

vi.mock('./useSnackbar', () => ({
  default: () => ({
    error: mockSnackbarError,
    success: vi.fn(),
    info: vi.fn(),
  }),
}))

vi.mock('./useRouter', () => ({
  default: () => ({
    name: mockRouterState.name,
    params: mockRouterState.params,
    meta: {},
    navigate: mockNavigate,
    navigateReplace: vi.fn(),
    setParams: vi.fn(),
    mergeParams: vi.fn(),
    replaceParams: vi.fn(),
    removeParams: vi.fn(),
  }),
}))

import useOrganizations from './useOrganizations'
import { SELECTED_ORG_STORAGE_KEY } from '../utils/localStorage'

const httpError = (status: number) =>
  Object.assign(new Error(`Request failed with status code ${status}`), {
    response: { status },
  })

/** The signed-in user is a member of exactly one org. */
const MY_ORG = { id: 'org_mine', name: 'mr-tester-org1', member: true }

function setupOrgList(orgs: any[] = [MY_ORG]) {
  mockV1OrganizationsList.mockResolvedValue({ data: orgs })
  mockV1OrganizationsMembersDetail.mockResolvedValue({ data: [] })
  mockV1OrganizationsRolesDetail.mockResolvedValue({ data: [] })
  mockV1OrganizationsTeamsDetail.mockResolvedValue({ data: [] })
}

/** Render the hook and wait for the org list load to settle. */
async function renderInitialized() {
  const rendered = renderHook(() => useOrganizations())
  await rendered.result.current.loadOrganizations()
  await waitFor(() => expect(rendered.result.current.initialized).toBe(true))
  return rendered
}

describe('useOrganizations stale org recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockRouterState.name = 'org_chat'
    mockRouterState.params = {}
    setupOrgList()
    mockV1OrganizationsDetail.mockResolvedValue({ data: MY_ORG })
  })

  // The reported bug: signing in as a different user on a browser that still
  // had `selected_org=unmanned-org` dropped you into an org you have no
  // membership in. The page then fired a wall of 403/500s and the org switcher
  // rendered blank, with nothing to move you off it.
  it('leaves an org the user has no access to, and forgets it', async () => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'unmanned-org')
    mockRouterState.params = { org_id: 'unmanned-org' }

    await renderInitialized()

    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith('org_chat', { org_id: 'mr-tester-org1' })
    )
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBe('mr-tester-org1')
    // We must never request details for an org we were told we can't see.
    expect(mockV1OrganizationsDetail).not.toHaveBeenCalled()
  })

  it('sends the user to the org picker when they have no usable org', async () => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'unmanned-org')
    mockRouterState.params = { org_id: 'unmanned-org' }
    setupOrgList([])

    await renderInitialized()

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('orgs'))
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBeNull()
  })

  it('stays put when the URL org is one of ours', async () => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'mr-tester-org1')
    mockRouterState.params = { org_id: 'mr-tester-org1' }

    await renderInitialized()

    await waitFor(() => expect(mockV1OrganizationsDetail).toHaveBeenCalledWith('org_mine'))
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBe('mr-tester-org1')
  })

  // A failed list load leaves `initialized` true (it is set in a `finally`)
  // with an empty `organizations`. That is "we don't know", not "you have no
  // orgs" — treating them the same evicts a user from an org they can fully
  // access whenever GET /organizations blips.
  it.each([500, 0])(
    'stays put when the org list request itself failed (%i)',
    async (status) => {
      localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'mr-tester-org1')
      mockRouterState.params = { org_id: 'mr-tester-org1' }
      mockV1OrganizationsList.mockRejectedValue(
        status === 0 ? new Error('Network Error') : httpError(status)
      )

      const { result } = await renderInitialized()

      expect(result.current.organizations).toEqual([])
      expect(mockNavigate).not.toHaveBeenCalled()
      expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBe('mr-tester-org1')
    }
  )

  // Once the list does load and genuinely contains nothing, the user really
  // has no orgs and belongs on the picker.
  it('recovers on the next successful load after a failed one', async () => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'unmanned-org')
    mockRouterState.params = { org_id: 'unmanned-org' }
    mockV1OrganizationsList.mockRejectedValueOnce(httpError(500))

    const rendered = renderHook(() => useOrganizations())
    await rendered.result.current.loadOrganizations()
    await waitFor(() => expect(rendered.result.current.initialized).toBe(true))
    expect(mockNavigate).not.toHaveBeenCalled()

    // Retry succeeds; only now do we know the org isn't ours.
    await rendered.result.current.loadOrganizations()
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith('org_chat', { org_id: 'mr-tester-org1' })
    )
  })

  it('never redirects out of an embed route', async () => {
    // Scoped embed keys see an empty org list by design.
    mockRouterState.name = 'embed_task'
    mockRouterState.params = { org_id: 'unmanned-org' }
    setupOrgList([])

    await renderInitialized()

    expect(mockNavigate).not.toHaveBeenCalled()
  })
})

describe('useOrganizations loadOrganization error handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockRouterState.name = 'org_chat'
    mockRouterState.params = { org_id: 'mr-tester-org1' }
    setupOrgList([MY_ORG, { id: 'org_other', name: 'other-org', member: true }])
  })

  // Membership can be revoked between the list load and the detail load.
  it.each([403, 404])(
    'forgets the org and leaves when the detail request answers %i',
    async (status) => {
      localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'mr-tester-org1')
      mockV1OrganizationsDetail.mockRejectedValue(httpError(status))

      const { result } = await renderInitialized()

      await waitFor(() => expect(mockNavigate).toHaveBeenCalled())
      expect(result.current.organization).toBeUndefined()
      // Recovery lands on a different org, so the remembered slug must change.
      expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).not.toBe('mr-tester-org1')
    }
  )

  // A transient failure must NOT eject the user — they should be able to retry
  // or wait for auth to recover.
  it.each([401, 500])('keeps the user in place on a transient %i', async (status) => {
    localStorage.setItem(SELECTED_ORG_STORAGE_KEY, 'mr-tester-org1')
    mockV1OrganizationsDetail.mockRejectedValue(httpError(status))

    await renderInitialized()

    await waitFor(() => expect(mockSnackbarError).toHaveBeenCalled())
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(localStorage.getItem(SELECTED_ORG_STORAGE_KEY)).toBe('mr-tester-org1')
  })
})
