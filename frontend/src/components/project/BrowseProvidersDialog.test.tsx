import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import BrowseProvidersDialog from './BrowseProvidersDialog'

const patMocks = vi.hoisted(() => ({
  connections: [] as Array<{ id: string; provider_type: string; username: string; base_url: string }>,
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  browseSaved: vi.fn(),
  browseRemote: vi.fn(),
}))

vi.mock('../../services/oauthProvidersService', () => ({
  oauthConnectionsQueryKey: () => ['oauth-connections'],
  oauthProvidersQueryKey: () => ['oauth-providers'],
  useListOAuthConnections: () => ({
    data: [{
      id: 'github-connection',
      scopes: ['repo', 'workflow', 'read:org'],
      profile: { name: 'Nessie' },
      provider: { id: 'github-provider', type: 'github', name: 'GitHub' },
    }],
    isLoading: false,
  }),
  useListOAuthProviders: () => ({
    data: [{
      id: 'github-provider',
      type: 'github',
      name: 'GitHub',
      enabled: true,
    }],
    isLoading: false,
  }),
  useListOAuthConnectionRepositories: () => ({
    data: {
      repositories: [{
        name: 'helix',
        full_name: 'helixml/helix',
        clone_url: 'https://github.com/helixml/helix.git',
      }],
    },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
}))

vi.mock('../../services/gitProviderConnectionService', () => ({
  useGitProviderConnections: () => ({ data: patMocks.connections, isLoading: false }),
  useCreateGitProviderConnection: () => ({ mutateAsync: patMocks.create, isPending: false }),
  useUpdateGitProviderConnection: () => ({ mutateAsync: patMocks.update, isPending: false }),
  useDeleteGitProviderConnection: () => ({ mutateAsync: patMocks.remove, isPending: false }),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1GitProviderConnectionsRepositoriesDetail: patMocks.browseSaved,
      v1GitBrowseRemoteCreate: patMocks.browseRemote,
    }),
    get: vi.fn(),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({ admin: false }),
}))

vi.mock('../../contexts/settingsDialog', () => ({
  useSettingsDialog: () => ({ openDialog: vi.fn() }),
}))

describe('BrowseProvidersDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    patMocks.connections = []
    patMocks.browseSaved.mockReset()
    patMocks.update.mockReset()
    patMocks.remove.mockReset()
  })

  const renderDialog = (onSelectRepository = vi.fn()) => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <BrowseProvidersDialog open onClose={vi.fn()} onSelectRepository={onSelectRepository} />
      </QueryClientProvider>,
    )
    return onSelectRepository
  }

  const saveGitLabConnection = () => {
    patMocks.connections = [{
      id: 'saved-gitlab',
      provider_type: 'gitlab',
      username: 'gitlab-user',
      base_url: 'https://gitlab.example.com',
    }]
  }

  it('opens repository picking immediately for a connected GitHub account', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <BrowseProvidersDialog
          open
          onClose={vi.fn()}
          onSelectRepository={vi.fn()}
        />
      </QueryClientProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: /GitHub/i }))

    expect(screen.getByText('Choose a GitHub repository')).toBeInTheDocument()
    expect(screen.queryByText('Choose how you want to connect to GitHub.')).not.toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveClass('MuiAlert-outlined')
    fireEvent.click(screen.getByText('helixml/helix'))
    expect(screen.queryByLabelText('Selected repository')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByText('Choose a Repository Source')).toBeInTheDocument()
  })

  it('makes saved token management reachable without browsing repositories', () => {
    saveGitLabConnection()
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Manage GitLab connection' }))
    expect(screen.getByRole('button', { name: 'Remove saved token' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Replace saved token' })).toBeInTheDocument()
    expect(patMocks.browseSaved).not.toHaveBeenCalled()
  })

  it('replaces an expired token on the same connection, then browses and links with its ID', async () => {
    saveGitLabConnection()
    const repo = { name: 'repo', full_name: 'gitlab-user/repo', clone_url: 'https://gitlab.example.com/repo.git' }
    patMocks.browseSaved.mockRejectedValueOnce({ response: { data: 'Token was revoked' } })
      .mockResolvedValueOnce({ data: { repositories: [repo] } })
    patMocks.update.mockResolvedValue({ id: 'saved-gitlab' })
    const onSelect = renderDialog()

    fireEvent.click(screen.getByRole('button', { name: /^GitLab/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Token was revoked')
    fireEvent.click(screen.getByRole('button', { name: 'Manage connection' }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace saved token' }))
    const input = screen.getByLabelText('New Personal Access Token')
    expect(input).toHaveValue('')
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('autocomplete', 'new-password')
    expect(screen.queryByLabelText('GitLab Base URL (optional)')).not.toBeInTheDocument()
    fireEvent.change(input, { target: { value: 'new-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Replace token' }))

    expect(await screen.findByText('gitlab-user/repo')).toBeInTheDocument()
    expect(patMocks.update).toHaveBeenCalledWith({ id: 'saved-gitlab', request: { token: 'new-token' } })
    expect(patMocks.create).not.toHaveBeenCalled()
    expect(patMocks.browseSaved).toHaveBeenNthCalledWith(2, 'saved-gitlab')
    fireEvent.click(screen.getByText('gitlab-user/repo'))
    fireEvent.click(screen.getByRole('button', { name: 'Link Repository' }))
    expect(onSelect).toHaveBeenCalledWith(repo, 'gitlab', undefined, 'saved-gitlab')
  })

  it('keeps a failed replacement on the empty-by-default token form and allows retry', async () => {
    saveGitLabConnection()
    patMocks.update.mockRejectedValueOnce({ response: { data: 'Invalid replacement token' } })
      .mockResolvedValueOnce({ id: 'saved-gitlab' })
    patMocks.browseSaved.mockResolvedValue({ data: { repositories: [{ name: 'repo', full_name: 'gitlab-user/repo' }] } })
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Manage GitLab connection' }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace saved token' }))
    fireEvent.change(screen.getByLabelText('New Personal Access Token'), { target: { value: 'invalid-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Replace token' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid replacement token')
    expect(patMocks.browseSaved).not.toHaveBeenCalled()
    expect(patMocks.remove).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('New Personal Access Token'), { target: { value: 'new-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Replace token' }))
    expect(await screen.findByText('gitlab-user/repo')).toBeInTheDocument()
    expect(patMocks.update).toHaveBeenLastCalledWith({ id: 'saved-gitlab', request: { token: 'new-token' } })
  })

  it('clears a typed replacement when returning to connection management', () => {
    saveGitLabConnection()
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Manage GitLab connection' }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace saved token' }))
    fireEvent.change(screen.getByLabelText('New Personal Access Token'), { target: { value: 'unsaved-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace saved token' }))
    expect(screen.getByLabelText('New Personal Access Token')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Replace token' })).toBeDisabled()
    expect(patMocks.update).not.toHaveBeenCalled()
  })

  it('removes the saved connection and allows entering a new token immediately', async () => {
    saveGitLabConnection()
    patMocks.remove.mockImplementation(async () => { patMocks.connections = [] })
    patMocks.browseRemote.mockResolvedValue({ data: { repositories: [{ name: 'new-repo', full_name: 'gitlab-user/new-repo' }] } })
    patMocks.create.mockResolvedValue({ id: 'new-gitlab' })
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Manage GitLab connection' }))
    fireEvent.click(screen.getByRole('button', { name: 'Remove saved token' }))
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect', exact: true }))
    await waitFor(() => expect(screen.queryByText('Disconnect Token')).not.toBeInTheDocument())
    expect(patMocks.remove).toHaveBeenCalledWith('saved-gitlab')
    expect(patMocks.browseSaved).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: /Use Personal Access Token/ }))
    expect(screen.getByLabelText('Personal Access Token')).toHaveValue('')
    fireEvent.change(screen.getByLabelText('Personal Access Token'), { target: { value: 'new-token' } })
    fireEvent.change(screen.getByLabelText('GitLab Base URL (optional)'), { target: { value: 'https://gitlab.example.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Browse Repositories' }))
    expect(await screen.findByText('gitlab-user/new-repo')).toBeInTheDocument()
    await waitFor(() => expect(patMocks.create).toHaveBeenCalledWith(expect.objectContaining({
      provider_type: 'gitlab', token: 'new-token', base_url: 'https://gitlab.example.com',
    })))
  })
})
