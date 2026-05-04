import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import Sandboxes from './Sandboxes'

// Regression: the URL carries the org *slug* ("/orgs/koala-bunny-corp/...")
// but API calls and sandbox.OrganizationID need the actual org id (org_xxx).
// Earlier code passed `router.params.org_id` straight through, which wrote the
// slug into the sandbox row and broke wallet lookups (GetWalletByOrg(slug) →
// not found) on billing/delete. These tests pin down that the page resolves
// the id from the account context instead.

const mockUseListSandboxes = vi.fn()
const mockUseDeleteSandbox = vi.fn(() => ({ mutateAsync: vi.fn() }))

vi.mock('../services/sandboxesService', () => ({
  useListSandboxes: (orgId: string | undefined) => mockUseListSandboxes(orgId),
  useDeleteSandbox: (orgId: string) => mockUseDeleteSandbox(orgId),
}))

const mockUseAccount = vi.fn()
vi.mock('../hooks/useAccount', () => ({
  default: () => mockUseAccount(),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    name: 'org_sandboxes',
    params: { org_id: 'koala-bunny-corp' },
    meta: {},
    navigate: vi.fn(),
    navigateReplace: vi.fn(),
    setParams: vi.fn(),
    mergeParams: vi.fn(),
    replaceParams: vi.fn(),
    removeParams: vi.fn(),
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn() }),
}))

vi.mock('../hooks/useViewMode', () => ({
  default: () => ['table', vi.fn()],
}))

// Stub Page + heavy children to keep the test focused on the orgId wiring.
vi.mock('../components/system/Page', () => ({
  default: ({ children }: any) => <div>{children}</div>,
}))
vi.mock('../components/widgets/LoadingSpinner', () => ({ default: () => null }))
vi.mock('../components/widgets/DeleteConfirmWindow', () => ({ default: () => null }))
vi.mock('../components/widgets/ViewModeToggle', () => ({ default: () => null }))
vi.mock('../components/sandboxes/CreateSandboxDialog', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxesView', () => ({ default: () => null }))

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Sandboxes />
    </QueryClientProvider>,
  )

describe('Sandboxes page orgId resolution', () => {
  beforeEach(() => {
    mockUseListSandboxes.mockReset()
    mockUseListSandboxes.mockReturnValue({ data: { sandboxes: [] }, isLoading: false })
    mockUseDeleteSandbox.mockClear()
  })

  it('passes the resolved organization id (not the URL slug) to useListSandboxes', () => {
    mockUseAccount.mockReturnValue({
      organizationTools: { organization: { id: 'org_real123', name: 'koala-bunny-corp' } },
    })
    renderPage()
    expect(mockUseListSandboxes).toHaveBeenCalledWith('org_real123')
    expect(mockUseListSandboxes).not.toHaveBeenCalledWith('koala-bunny-corp')
  })

  it('passes the resolved organization id to useDeleteSandbox', () => {
    mockUseAccount.mockReturnValue({
      organizationTools: { organization: { id: 'org_real123', name: 'koala-bunny-corp' } },
    })
    renderPage()
    expect(mockUseDeleteSandbox).toHaveBeenCalledWith('org_real123')
  })

  it('passes empty string while the organization context is still loading', () => {
    // Race: account context not yet hydrated. We must NOT fall back to the
    // slug — the API would silently 404 / write the slug into the row.
    mockUseAccount.mockReturnValue({
      organizationTools: { organization: undefined },
    })
    renderPage()
    expect(mockUseListSandboxes).toHaveBeenCalledWith(undefined)
    expect(mockUseDeleteSandbox).toHaveBeenCalledWith('')
  })
})
