import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import SandboxDetail from './SandboxDetail'

// Regression test for the org-slug-vs-id mix-up in SandboxDetail. The URL
// carries the slug; useSandbox / useDeleteSandbox must receive the resolved
// org id from the account context. See Sandboxes.test.tsx for the matching
// test on the list page.

const mockUseSandbox = vi.fn()
const mockUseDeleteSandbox = vi.fn((_orgId: string) => ({ mutateAsync: vi.fn() }))

vi.mock('../services/sandboxesService', () => ({
  useSandbox: (orgId: string | undefined, sandboxId: string | undefined) =>
    mockUseSandbox(orgId, sandboxId),
  useDeleteSandbox: (orgId: string) => mockUseDeleteSandbox(orgId),
}))

const mockUseAccount = vi.fn()
vi.mock('../hooks/useAccount', () => ({
  default: () => mockUseAccount(),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    name: 'org_sandbox_detail',
    params: { org_id: 'koala-bunny-corp', sandbox_id: 'sbx_test' },
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

vi.mock('../hooks/useUrlTab', () => ({
  default: () => ['overview', vi.fn()],
}))

vi.mock('../components/system/Page', () => ({
  default: ({ children }: any) => <div>{children}</div>,
}))
vi.mock('../components/widgets/PageLoader', () => ({ default: () => null }))
vi.mock('../components/widgets/DeleteConfirmWindow', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxDesktopTab', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxOverviewTab', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxCommandsTab', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxFilesTab', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxTerminal', () => ({ default: () => null }))
vi.mock('../components/sandboxes/SandboxStatusBadge', () => ({ default: () => null }))

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SandboxDetail />
    </QueryClientProvider>,
  )

describe('SandboxDetail page orgId resolution', () => {
  beforeEach(() => {
    mockUseSandbox.mockReset()
    mockUseSandbox.mockReturnValue({
      data: { id: 'sbx_test', status: 'running', runtime: 'headless-ubuntu' },
      isLoading: false,
    })
    mockUseDeleteSandbox.mockClear()
  })

  it('passes the resolved organization id (not the URL slug) to useSandbox', () => {
    mockUseAccount.mockReturnValue({
      organizationTools: { organization: { id: 'org_real123', name: 'koala-bunny-corp' } },
    })
    renderPage()
    expect(mockUseSandbox).toHaveBeenCalledWith('org_real123', 'sbx_test')
  })

  it('passes the resolved organization id to useDeleteSandbox', () => {
    mockUseAccount.mockReturnValue({
      organizationTools: { organization: { id: 'org_real123', name: 'koala-bunny-corp' } },
    })
    renderPage()
    expect(mockUseDeleteSandbox).toHaveBeenCalledWith('org_real123')
    expect(mockUseDeleteSandbox).not.toHaveBeenCalledWith('koala-bunny-corp')
  })
})
