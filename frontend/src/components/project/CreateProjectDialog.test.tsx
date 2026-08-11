import React, { forwardRef, useImperativeHandle } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { TypesGitRepository } from '../../api/api'
import CreateProjectDialog from './CreateProjectDialog'

const mocks = vi.hoisted(() => ({
  createAgent: vi.fn(),
  createProject: vi.fn(),
  createRepo: vi.fn(),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    organizationTools: {
      orgID: 'test-org',
      organization: { id: 'org-1', name: 'test-org' },
    },
    orgNavigate: vi.fn(),
  }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({ get: vi.fn() }),
}))

vi.mock('../../services', () => ({
  useCreateProject: () => ({
    isPending: false,
    mutateAsync: mocks.createProject,
  }),
}))

vi.mock('../../services/oauthProvidersService', () => ({
  oauthConnectionsQueryKey: () => ['oauth-connections'],
  useListOAuthConnections: () => ({ data: [] }),
  useListOAuthProviders: () => ({ data: [] }),
  useListOAuthConnectionRepositories: () => ({
    data: { repositories: [] },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
}))

vi.mock('../agent/CodingAgentForm', () => ({
  default: forwardRef(function MockCodingAgentForm(_, ref) {
    useImperativeHandle(ref, () => ({ handleCreateAgent: mocks.createAgent }))
    return <div>Runtime selector</div>
  }),
}))

vi.mock('./BrowseProvidersDialog', () => ({
  default: () => null,
}))

function renderDialog(repositories: TypesGitRepository[] = []) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <CreateProjectDialog
        open
        onClose={vi.fn()}
        repositories={repositories}
        onCreateRepo={mocks.createRepo}
      />
    </QueryClientProvider>,
  )
}

describe('CreateProjectDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.createAgent.mockResolvedValue({ id: 'agent-1' })
    mocks.createProject.mockResolvedValue({ id: 'project-1' })
    mocks.createRepo.mockResolvedValue({ id: 'repo-1', name: 'demo' })
  })

  it('disables existing repository selection when none are available', async () => {
    renderDialog()

    const useExisting = screen.getByRole('button', { name: /Existing repository/i })
    expect(useExisting).toBeDisabled()

    fireEvent.mouseOver(useExisting.parentElement as HTMLElement)
    expect(await screen.findByText("You don't have any repositories yet.")).toBeInTheDocument()
  })

  it('puts repository selection before the name field', () => {
    renderDialog()

    const repositoryHeading = screen.getByText('Repository')
    const nameField = screen.getByRole('textbox', { name: /^Name$/i })

    expect(
      repositoryHeading.compareDocumentPosition(nameField)
        & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(screen.queryByRole('textbox', { name: /Project Name/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /Repository Name/i })).not.toBeInTheDocument()
    expect(screen.getByLabelText('Supported Git providers').querySelectorAll('svg')).toHaveLength(3)
  })

  it('creates the project and repository with the same name', async () => {
    renderDialog()

    fireEvent.change(screen.getByRole('textbox', { name: /^Name$/i }), {
      target: { value: '  Demo project  ' },
    })

    const createProject = screen.getByRole('button', { name: /Create Project/i })
    expect(createProject).toHaveTextContent(/(⌘|Ctrl)Enter/)
    await waitFor(() => expect(createProject).toBeEnabled())

    fireEvent.click(createProject)

    await waitFor(() => expect(mocks.createRepo).toHaveBeenCalledWith('Demo project', ''))
    await waitFor(() => expect(mocks.createAgent).toHaveBeenCalledOnce())
    expect(mocks.createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Demo project',
      default_repo_id: 'repo-1',
      default_helix_app_id: 'agent-1',
    }))
  })

  it('submits with Cmd or Ctrl plus Enter', async () => {
    renderDialog()

    fireEvent.change(screen.getByRole('textbox', { name: /^Name$/i }), {
      target: { value: 'Shortcut project' },
    })
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Enter', metaKey: true })

    await waitFor(() => expect(mocks.createRepo).toHaveBeenCalledWith('Shortcut project', ''))
    await waitFor(() => expect(mocks.createProject).toHaveBeenCalledOnce())
  })
})
