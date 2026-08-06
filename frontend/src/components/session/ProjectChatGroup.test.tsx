import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TypesSpecTaskStatus } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import ProjectChatGroup from './ProjectChatGroup'

const mocks = vi.hoisted(() => ({
  emptySessions: false,
  tasks: [] as SpecTask[],
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../../services/sessionService', () => ({
  useListSessions: (...args: unknown[]) => {
    const pageSize = args[5] as number
    const sessions = mocks.emptySessions
      ? []
      : Array.from({ length: pageSize }, (_, index) => ({
        session_id: `session-${index + 1}`,
        name: `Session ${index + 1}`,
        updated: new Date(Date.UTC(2026, 7, 6, 12, 0, -index)).toISOString(),
      }))
    return {
      data: { data: { sessions, totalCount: mocks.emptySessions ? 0 : 40 } },
      isLoading: false,
      isFetching: false,
      isError: false,
    }
  },
}))

vi.mock('../../services/specTaskService', () => ({
  useSpecTasks: () => ({
    data: mocks.tasks,
    isLoading: false,
    isFetching: false,
    isError: false,
  }),
}))

afterEach(() => {
  mocks.emptySessions = false
  mocks.tasks = []
})

const renderEmptyProject = (collapsed = false) => render(
  <ProjectChatGroup
    orgId="org-one"
    project={{ id: 'project-one', name: 'Empty project' }}
    collapsed={collapsed}
    query=""
    activeItemId=""
    relativeTimeNow={Date.now()}
    enabled
    archivingItemId={null}
    onToggle={vi.fn()}
    onNewTask={vi.fn()}
    onOpenItem={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

describe('ProjectChatGroup', () => {
  it('renders an expanded project with no tasks', () => {
    mocks.emptySessions = true
    renderEmptyProject()

    expect(screen.getByText('Empty project')).toBeInTheDocument()
    expect(screen.getByText('No tasks yet')).toBeInTheDocument()
  })

  it('keeps an empty collapsed project visible without the empty-state row', () => {
    mocks.emptySessions = true
    renderEmptyProject(true)

    expect(screen.getByText('Empty project')).toBeInTheDocument()
    expect(screen.queryByText('No tasks yet')).not.toBeInTheDocument()
  })
})

describe('ProjectChatGroup pagination', () => {
  it('shows less after expansion and restores the initial item count', () => {
    render(
      <ProjectChatGroup
        orgId="org-test"
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={vi.fn()}
        onArchiveItem={vi.fn()}
      />,
    )

    expect(screen.getByText('Session 6')).toBeInTheDocument()
    expect(screen.queryByText('Session 7')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Show less' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show more' }))

    expect(screen.getByText('Session 7')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show less' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show less' }))

    expect(screen.getByText('Session 6')).toBeInTheDocument()
    expect(screen.queryByText('Session 7')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Show less' })).not.toBeInTheDocument()
  })

  it('explains the neutral status when the sandbox and agent are offline', async () => {
    const onOpenItem = vi.fn()
    mocks.tasks = [{
      id: 'offline-task',
      project_id: 'project-test',
      name: 'Offline task',
      status: TypesSpecTaskStatus.TaskStatusDone,
      sandbox_state: 'absent',
      repo_pull_requests: [{
        pr_state: 'open',
        pr_url: 'https://github.com/helixml/helix/pull/1',
      }],
    }]

    render(
      <ProjectChatGroup
        orgId="org-test"
        project={{ id: 'project-test', name: 'Project Test' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={onOpenItem}
        onArchiveItem={vi.fn()}
      />,
    )

    const pullRequestLink = screen.getByRole('link', { name: 'Pull request is open' })
    expect(pullRequestLink).toHaveAttribute('href', 'https://github.com/helixml/helix/pull/1')
    expect(pullRequestLink).toHaveAttribute('target', '_blank')
    expect(pullRequestLink).toHaveStyle({ color: '#10b981' })

    fireEvent.click(pullRequestLink)
    expect(onOpenItem).not.toHaveBeenCalled()

    fireEvent.mouseOver(pullRequestLink)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Pull request is open')
    fireEvent.mouseLeave(pullRequestLink)
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument())

    fireEvent.mouseOver(screen.getByText('Completed'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Sandbox and agent are offline')
  })
})
