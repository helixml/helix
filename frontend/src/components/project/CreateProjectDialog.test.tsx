import React, { forwardRef, useImperativeHandle } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { TypesGitRepository } from '../../api/api'
import CreateProjectDialog from './CreateProjectDialog'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  createProject: vi.fn(),
  createRepo: vi.fn(),
  projects: [] as Array<{ id: string; name: string }>,
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    organizationTools: {
      orgID: 'test-org',
      organization: { id: 'org-1', name: 'test-org' },
    },
    user: { id: 'user-1' },
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
  useListProjects: () => ({
    data: mocks.projects,
    isLoading: false,
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
    useImperativeHandle(ref, () => ({ handleGetConfig: mocks.getConfig }))
    return <div>Runtime selector</div>
  }),
}))

vi.mock('./BrowseProvidersDialog', () => ({
  default: () => null,
}))

function renderDialog(
  repositories: TypesGitRepository[] = [],
  onClose = vi.fn(),
  onSuccess = vi.fn(),
) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <CreateProjectDialog
        open
        onClose={onClose}
        onSuccess={onSuccess}
        repositories={repositories}
        onCreateRepo={mocks.createRepo}
      />
    </QueryClientProvider>,
  )
}

describe('CreateProjectDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getConfig.mockReturnValue({
      runtime: 'claude_code',
      credential_type: 'subscription',
      model: 'claude-opus-5',
    })
    mocks.createProject.mockResolvedValue({ id: 'project-1' })
    mocks.createRepo.mockResolvedValue({ id: 'repo-1', name: 'demo' })
    mocks.projects.length = 0
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
    expect(mocks.createProject).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Demo project',
      default_repo_id: 'repo-1',
      code_agent_config: expect.objectContaining({
        runtime: 'claude_code',
        credential_type: 'subscription',
        model: 'claude-opus-5',
      }),
    }))
  })

  it('shows subscription policy failures without closing the dialog', async () => {
    const onClose = vi.fn()
    const onSuccess = vi.fn()
    mocks.createProject.mockRejectedValue({
      response: {
        data: 'subscription credentials are not enabled for coding-agent harness "claude_code" in this organization',
      },
    })
    renderDialog([], onClose, onSuccess)

    fireEvent.change(screen.getByRole('textbox', { name: /^Name$/i }), {
      target: { value: 'Demo project' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Create Project/i }))

    expect(await screen.findByText(
      'Claude Code subscription access is not enabled for this organization. Ask an organization administrator to enable it in Settings > Providers, or select another coding agent.',
    )).toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
    expect(onSuccess).not.toHaveBeenCalled()
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

  it('shows an inline error and blocks an existing project name', async () => {
    mocks.projects.push({ id: 'project-existing', name: 'Demo Project' })
    renderDialog()

    const nameField = screen.getByRole('textbox', { name: /^Name$/i })
    fireEvent.change(nameField, { target: { value: ' demo project ' } })

    expect(await screen.findByText('A project named “demo project” already exists.')).toBeInTheDocument()
    expect(nameField).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('button', { name: /Create Project/i })).toBeDisabled()
    expect(mocks.createRepo).not.toHaveBeenCalled()
  })

  it('suffixes a conflicting Helix repository name without changing the project name', async () => {
    renderDialog([
      { id: 'repo-existing', name: 'Demo', repo_type: 'code' } as TypesGitRepository,
    ])

    fireEvent.change(screen.getByRole('textbox', { name: /^Name$/i }), {
      target: { value: 'demo' },
    })

    expect(await screen.findByText('Creates project “demo” and Helix-hosted repository “demo-2”.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Create Project/i }))

    await waitFor(() => expect(mocks.createRepo).toHaveBeenCalledWith('demo-2', ''))
    expect(mocks.createProject).toHaveBeenCalledWith(expect.objectContaining({ name: 'demo' }))
  })
})
