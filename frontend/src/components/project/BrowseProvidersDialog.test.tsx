import React from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import BrowseProvidersDialog from './BrowseProvidersDialog'

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
  useGitProviderConnections: () => ({ data: [], isLoading: false }),
  useCreateGitProviderConnection: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteGitProviderConnection: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({ getApiClient: vi.fn(), get: vi.fn() }),
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
})
