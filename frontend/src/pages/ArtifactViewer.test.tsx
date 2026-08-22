import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ArtifactViewer from './ArtifactViewer'

const mocks = vi.hoisted(() => ({
  account: { initialized: true, user: undefined as undefined | { id: string } },
  artifactKind: 'single_file',
  navigate: vi.fn(),
  mutateAsync: vi.fn(),
  pageProps: undefined as undefined | {
    breadcrumbTitle?: string
    breadcrumbs?: Array<{ title: string, routeName?: string, params?: Record<string, string> }>
  },
}))

vi.mock('../hooks/useAccount', () => ({
  default: () => mocks.account,
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    params: { artifact_id: 'art_public' },
    navigate: mocks.navigate,
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('../services/artifactService', () => ({
  useGetArtifactViewer: () => ({
    data: {
      id: 'art_public',
      project_id: 'prj_test',
      project_name: 'Public project',
      organization_id: 'org_test',
      organization_name: 'example-org',
      name: 'Public artifact',
      kind: mocks.artifactKind,
      visibility: 'public',
      active_version_id: 'artv_test',
      subdomain_url: 'http://artifact.example.test/',
      can_edit: !!mocks.account.user,
    },
    isLoading: false,
    error: null,
  }),
  useSetArtifactVisibility: () => ({
    mutateAsync: mocks.mutateAsync,
    isPending: false,
  }),
}))

vi.mock('../components/system/Page', () => ({
  default: (props: {
    topbarContent: React.ReactNode
    children: React.ReactNode
    breadcrumbTitle?: string
    breadcrumbs?: Array<{ title: string, routeName?: string, params?: Record<string, string> }>
  }) => {
    mocks.pageProps = props
    return (
      <div>
        <header>{props.topbarContent}</header>
        <main>{props.children}</main>
      </div>
    )
  },
}))

describe('ArtifactViewer', () => {
  beforeEach(() => {
    mocks.account.user = undefined
    mocks.artifactKind = 'single_file'
    mocks.navigate.mockReset()
    mocks.mutateAsync.mockReset()
    mocks.pageProps = undefined
    localStorage.clear()
  })

  afterEach(() => vi.clearAllMocks())

  it('renders a public artifact with Login instead of Share when signed out', () => {
    render(<ArtifactViewer />)

    expect(screen.getByTitle('Public artifact')).toHaveAttribute('src', 'http://artifact.example.test/')
    expect(screen.getByRole('button', { name: 'Login' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Share' })).not.toBeInTheDocument()
    expect(mocks.pageProps?.breadcrumbTitle).toBe('Public artifact')
    expect(mocks.pageProps?.breadcrumbs?.at(-1)).toEqual({
      title: 'artifacts',
      routeName: 'org_project-artifacts',
      params: { org_id: 'example-org', id: 'prj_test' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Login' }))
    expect(localStorage.getItem('login_redirect_url')).toBe('/')
    expect(mocks.navigate).toHaveBeenCalledWith('login')
  })

  it('renders Share for an authenticated editor', () => {
    mocks.account.user = { id: 'usr_test' }
    render(<ArtifactViewer />)

    expect(screen.getByRole('button', { name: 'Share' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Login' })).not.toBeInTheDocument()
  })

  it('renders image artifacts as a contained image', () => {
    mocks.artifactKind = 'image'
    render(<ArtifactViewer />)

    expect(screen.getByRole('img', { name: 'Public artifact' })).toHaveAttribute('src', 'http://artifact.example.test/')
    expect(screen.queryByTitle('Public artifact')).not.toBeInTheDocument()
  })

  it('renders PDF artifacts in the browser frame', () => {
    mocks.artifactKind = 'pdf'
    render(<ArtifactViewer />)

    expect(screen.getByTitle('Public artifact')).toHaveAttribute('src', '/artifacts/art_public/document')
    expect(screen.getByTitle('Public artifact')).not.toHaveAttribute('sandbox')
  })
})
