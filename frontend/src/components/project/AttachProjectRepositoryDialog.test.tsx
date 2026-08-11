import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AttachProjectRepositoryDialog from './AttachProjectRepositoryDialog'

const mocks = vi.hoisted(() => ({
  attachRepository: vi.fn(),
  createRepository: vi.fn(),
  snackbarSuccess: vi.fn(),
  snackbarError: vi.fn(),
  repositories: [] as Array<Record<string, unknown>>,
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    user: { id: 'user-1' },
    organizationTools: {
      organization: { id: 'org-1', name: 'test-org', display_name: 'Test Org' },
    },
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({
    success: mocks.snackbarSuccess,
    error: mocks.snackbarError,
  }),
}))

vi.mock('../../services', () => ({
  useAttachRepositoryToProject: () => ({
    mutateAsync: mocks.attachRepository,
    isPending: false,
  }),
}))

vi.mock('../../services/gitRepositoryService', () => ({
  useGitRepositories: () => ({
    data: mocks.repositories,
    isLoading: false,
  }),
  useCreateGitRepository: () => ({
    mutateAsync: mocks.createRepository,
    isPending: false,
  }),
}))

vi.mock('./BrowseProvidersDialog', () => ({
  default: (props: any) => (
    <div>
      <div>{props.repositorySourceLabel}</div>
      <div>
        {props.helixRepositories.map((repository: { name: string }) => repository.name).join(',')}
      </div>
      <button
        onClick={() => props.onSelectHelixRepository(props.helixRepositories[0])}
      >
        Select existing
      </button>
      <button
        onClick={() => props.onSelectRepository(
          {
            name: 'external-repo',
            full_name: 'helixml/external-repo',
            clone_url: 'https://github.com/helixml/external-repo.git',
            default_branch: 'main',
          },
          'github',
          'oauth-1',
        )}
      >
        Select external
      </button>
    </div>
  ),
}))

describe('AttachProjectRepositoryDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.repositories = [
      { id: 'attached-repo', name: 'already-attached', repo_type: 'code' },
      { id: 'existing-repo', name: 'existing', repo_type: 'code' },
    ]
    mocks.attachRepository.mockResolvedValue(undefined)
    mocks.createRepository.mockResolvedValue({ id: 'linked-repo' })
  })

  it('attaches an existing repository without relinking it', async () => {
    const onClose = vi.fn()
    render(
      <AttachProjectRepositoryDialog
        open
        onClose={onClose}
        projectId="project-1"
        attachedRepositories={[{ id: 'attached-repo' }]}
      />,
    )

    expect(screen.getByText('Existing repositories')).toBeInTheDocument()
    expect(screen.getByText('existing')).toBeInTheDocument()
    expect(screen.queryByText('already-attached')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Select existing' }))

    await waitFor(() => {
      expect(mocks.attachRepository).toHaveBeenCalledWith('existing-repo')
    })
    expect(mocks.createRepository).not.toHaveBeenCalled()
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('links an external repository and then attaches it', async () => {
    const onClose = vi.fn()
    render(
      <AttachProjectRepositoryDialog
        open
        onClose={onClose}
        projectId="project-1"
        attachedRepositories={[]}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Select external' }))

    await waitFor(() => {
      expect(mocks.createRepository).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'external-repo',
          organization_id: 'org-1',
          owner_id: 'user-1',
          is_external: true,
          external_url: 'https://github.com/helixml/external-repo.git',
          oauth_connection_id: 'oauth-1',
        }),
      )
      expect(mocks.attachRepository).toHaveBeenCalledWith('linked-repo')
    })
    expect(onClose).toHaveBeenCalledOnce()
  })
})
