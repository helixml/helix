import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ProjectChatItemContextMenu from './ProjectChatItemContextMenu'

const mocks = vi.hoisted(() => ({
  renameSession: vi.fn(),
  updateSpecTask: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  resumeSession: vi.fn(),
  stopExternalAgent: vi.fn(),
  invalidateSpecTaskStatusQueries: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: mocks.success, error: mocks.error, info: mocks.info }),
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1SessionsResumeCreate: mocks.resumeSession,
      v1SessionsStopExternalAgentDelete: mocks.stopExternalAgent,
    }),
  }),
}))

vi.mock('../../services/sessionService', () => ({
  GET_SESSION_QUERY_KEY: (id: string) => ['session', id],
  useRenameSession: () => ({ mutateAsync: mocks.renameSession }),
}))

vi.mock('../../services/specTaskService', () => ({
  invalidateSpecTaskStatusQueries: mocks.invalidateSpecTaskStatusQueries,
  useUpdateSpecTask: () => ({ mutateAsync: mocks.updateSpecTask }),
}))

afterEach(() => {
  vi.clearAllMocks()
})

const renderContextMenu = (component: ReactElement) => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      {component}
    </QueryClientProvider>,
  )
}

describe('ProjectChatItemContextMenu', () => {
  it('renames a spec task through its user title override', async () => {
    mocks.updateSpecTask.mockResolvedValue({})
    const onClose = vi.fn()
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{ id: 'task-one', kind: 'spec-task', title: 'Old task name' }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={onClose}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename' }))
    expect(onClose).toHaveBeenCalledOnce()
    expect(screen.getByRole('dialog', { name: 'Rename task' })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '  New task name  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => expect(mocks.updateSpecTask).toHaveBeenCalledWith({
      taskId: 'task-one',
      updates: { user_short_title: 'New task name' },
    }))
    expect(mocks.renameSession).not.toHaveBeenCalled()
    expect(mocks.success).toHaveBeenCalledWith('Task renamed')
  })

  it('renames a normal chat session', async () => {
    mocks.renameSession.mockResolvedValue({})
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{ id: 'session-one', kind: 'session', title: 'Old chat name' }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename' }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New chat name' } })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => expect(mocks.renameSession).toHaveBeenCalledWith({
      sessionId: 'session-one',
      name: 'New chat name',
    }))
    expect(mocks.updateSpecTask).not.toHaveBeenCalled()
    expect(mocks.success).toHaveBeenCalledWith('Chat renamed')
  })

  it('opens project board, settings, and artifacts for items in a project', () => {
    const onOpenProjectBoard = vi.fn()
    const onOpenProjectSettings = vi.fn()
    const onOpenProjectArtifacts = vi.fn()
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{ id: 'session-one', kind: 'session', title: 'Chat', projectId: 'prj-1' }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
        onOpenProjectBoard={onOpenProjectBoard}
        onOpenProjectSettings={onOpenProjectSettings}
        onOpenProjectArtifacts={onOpenProjectArtifacts}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Project board' }))
    expect(onOpenProjectBoard).toHaveBeenCalledWith('prj-1')

    fireEvent.click(screen.getByRole('menuitem', { name: 'Project settings' }))
    expect(onOpenProjectSettings).toHaveBeenCalledWith('prj-1')

    fireEvent.click(screen.getByRole('menuitem', { name: 'Project artifacts' }))
    expect(onOpenProjectArtifacts).toHaveBeenCalledWith('prj-1')
  })

  it('hides project actions for items without a project', () => {
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{ id: 'session-one', kind: 'session', title: 'Chat' }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
        onOpenProjectBoard={vi.fn()}
        onOpenProjectSettings={vi.fn()}
        onOpenProjectArtifacts={vi.fn()}
      />,
    )

    expect(screen.queryByRole('menuitem', { name: 'Project board' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Project settings' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Project artifacts' })).not.toBeInTheDocument()
  })

  it('starts the sandbox of a stopped spec task', async () => {
    mocks.resumeSession.mockResolvedValue({})
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{
          id: 'task-one',
          kind: 'spec-task',
          title: 'Task',
          task: { id: 'task-one', sandbox_state: 'absent', planning_session_id: 'ses-1' } as any,
        }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Start sandbox' }))

    await waitFor(() => expect(mocks.resumeSession).toHaveBeenCalledWith('ses-1'))
    expect(mocks.stopExternalAgent).not.toHaveBeenCalled()
    expect(mocks.invalidateSpecTaskStatusQueries).toHaveBeenCalled()
    expect(mocks.success).toHaveBeenCalledWith('Sandbox starting')
  })

  it('stops the sandbox of a running external-agent chat', async () => {
    mocks.stopExternalAgent.mockResolvedValue({})
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{
          id: 'ses-2',
          kind: 'session',
          title: 'Chat',
          session: {
            session_id: 'ses-2',
            metadata: {
              agent_type: 'zed_external',
              container_name: 'zed-ses-2',
              external_agent_status: 'running',
            },
          } as any,
        }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Stop sandbox' }))

    await waitFor(() => expect(mocks.stopExternalAgent).toHaveBeenCalledWith('ses-2'))
    expect(mocks.resumeSession).not.toHaveBeenCalled()
    expect(mocks.success).toHaveBeenCalledWith('Sandbox stopped')
  })

  it('offers no sandbox action for a plain chat session', () => {
    renderContextMenu(
      <ProjectChatItemContextMenu
        item={{
          id: 'ses-3',
          kind: 'session',
          title: 'Chat',
          session: { session_id: 'ses-3', metadata: {} } as any,
        }}
        position={{ mouseX: 50, mouseY: 80 }}
        onClose={vi.fn()}
      />,
    )

    expect(screen.queryByRole('menuitem', { name: 'Start sandbox' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Stop sandbox' })).not.toBeInTheDocument()
  })
})
