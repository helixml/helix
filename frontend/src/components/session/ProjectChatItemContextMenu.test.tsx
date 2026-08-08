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
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: mocks.success, error: mocks.error }),
}))

vi.mock('../../services/sessionService', () => ({
  useRenameSession: () => ({ mutateAsync: mocks.renameSession }),
}))

vi.mock('../../services/specTaskService', () => ({
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
})
