import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TypesSpecTaskStatus } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import ProjectChatGroup from './ProjectChatGroup'

const mocks = vi.hoisted(() => ({
  emptySessions: false,
  tasks: [] as SpecTask[],
  sessionOptions: [] as any[],
  taskOptions: [] as any[],
}))

vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...await importOriginal<typeof import('@tanstack/react-query')>(),
  useQueries: () => [],
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../../services/sessionService', () => ({
  useListSessions: (...args: unknown[]) => {
    const pageSize = args[4] as number
    mocks.sessionOptions.push(args[5])
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
  useSpecTasks: (options: any) => {
    mocks.taskOptions.push(options)
    return {
      data: mocks.tasks,
      isLoading: false,
      isFetching: false,
      isError: false,
    }
  },
}))

vi.mock('../../services/projectService', () => ({
  useGetProjectRepositories: () => ({ data: [] }),
}))

afterEach(() => {
  mocks.emptySessions = false
  mocks.tasks = []
  mocks.sessionOptions = []
  mocks.taskOptions = []
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
    participantIds={[]}
    organizationMembers={[]}
    archivingItemId={null}
    onToggle={vi.fn()}
    onNewTask={vi.fn()}
    onOpenItem={vi.fn()}
    onOpenItemContextMenu={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

const renderAccessibleEmptyProject = (archived = false) => render(
  <ProjectChatGroup
    orgId="org-one"
    project={{ id: 'project-one', name: 'Accessible empty project', user_id: 'project-owner' }}
    collapsed={false}
    query=""
    activeItemId=""
    relativeTimeNow={Date.now()}
    enabled
    participantIds={[]}
    organizationMembers={[]}
    currentUser={{ id: 'project-member' }}
    archived={archived}
    archivingItemId={null}
    onToggle={vi.fn()}
    onNewTask={vi.fn()}
    onOpenItem={vi.fn()}
    onOpenItemContextMenu={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

describe('ProjectChatGroup', () => {
  it('shows an expanded empty project the user can access', async () => {
    mocks.emptySessions = true
    renderEmptyProject()

    await waitFor(() => expect(screen.getByText('Empty project')).toBeInTheDocument())
  })

  it('shows a collapsed empty project after probing its visible items', async () => {
    mocks.emptySessions = true
    renderEmptyProject(true)

    await waitFor(() => expect(screen.getByText('Empty project')).toBeInTheDocument())
    expect(mocks.sessionOptions.at(-1)?.enabled).toBe(true)
    expect(mocks.taskOptions.at(-1)?.enabled).toBe(true)
  })

  it('shows an empty project owned by another user when the current user has access', async () => {
    mocks.emptySessions = true
    renderAccessibleEmptyProject()

    await waitFor(() => expect(screen.getByText('Accessible empty project')).toBeInTheDocument())
  })

  it('hides a project with no archived sessions or tasks in the archived view', async () => {
    mocks.emptySessions = true
    renderAccessibleEmptyProject(true)

    await waitFor(() => expect(screen.queryByText('Accessible empty project')).not.toBeInTheDocument())
  })

  it('stops querying after a collapsed group proves it has visible items', async () => {
    renderEmptyProject(true)

    expect(mocks.sessionOptions.some((options) => options.enabled === true)).toBe(true)
    expect(mocks.taskOptions.some((options) => options.enabled === true)).toBe(true)
    await waitFor(() => expect(mocks.sessionOptions.at(-1)?.enabled).toBe(false))
    expect(mocks.taskOptions.at(-1)?.enabled).toBe(false)
  })

  it('queries once expanded', () => {
    renderEmptyProject(false)

    expect(mocks.sessionOptions.some((options) => options.enabled === true)).toBe(true)
    expect(mocks.taskOptions.some((options) => options.enabled === true)).toBe(true)
  })

  it('shows no item count, because a group only ever holds the page it fetched', () => {
    renderEmptyProject(false)

    expect(screen.queryByText('6+')).not.toBeInTheDocument()
    expect(screen.queryByText('7')).not.toBeInTheDocument()
  })

  it('opens the item context menu without navigating', () => {
    const onOpenItem = vi.fn()
    const onOpenItemContextMenu = vi.fn()
    render(
      <ProjectChatGroup
        orgId="org-test"
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={onOpenItem}
        onOpenItemContextMenu={onOpenItemContextMenu}
        onArchiveItem={vi.fn()}
      />,
    )

    const row = screen.getByText('Session 1').closest('[role="button"]')
    expect(row).not.toBeNull()
    fireEvent.contextMenu(row!)

    expect(onOpenItemContextMenu).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ id: 'session-1', kind: 'session' }),
    )
    expect(onOpenItem).not.toHaveBeenCalled()
  })

  it('opens the project context menu without collapsing the group', () => {
    const onToggle = vi.fn()
    const onOpenProjectContextMenu = vi.fn()
    render(
      <ProjectChatGroup
        orgId="org-test"
        project={{ id: 'project-test', name: 'Project Test' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={onToggle}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onOpenProjectContextMenu={onOpenProjectContextMenu}
        onArchiveItem={vi.fn()}
      />,
    )

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Project Test' }))

    expect(onOpenProjectContextMenu).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ id: 'project-test' }),
    )
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('starts a new chat when the project name is clicked, rather than collapsing', () => {
    const onToggle = vi.fn()
    const onNewTask = vi.fn()
    render(
      <ProjectChatGroup
        orgId="org-test"
        project={{ id: 'project-test', name: 'Project Test' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={onToggle}
        onNewTask={onNewTask}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onArchiveItem={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'New task in Project Test' }))

    expect(onNewTask).toHaveBeenCalled()
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('collapses from the chevron, and only from the chevron', () => {
    const onToggle = vi.fn()
    const onNewTask = vi.fn()
    render(
      <ProjectChatGroup
        orgId="org-test"
        project={{ id: 'project-test', name: 'Project Test' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={onToggle}
        onNewTask={onNewTask}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onArchiveItem={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Collapse Project Test' }))

    expect(onToggle).toHaveBeenCalledTimes(1)
    expect(onNewTask).not.toHaveBeenCalled()
  })

  it('keeps the name collapsing an archived group, which has no new task', () => {
    const onToggle = vi.fn()
    render(
      <ProjectChatGroup
        orgId="org-test"
        project={{ id: 'project-test', name: 'Project Test' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        archived
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={onToggle}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onArchiveItem={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Project Test' }))

    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('shows a pin indicator next to a pinned chat', () => {
    render(
      <ProjectChatGroup
        orgId="org-test"
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        pinnedChats={[{
          id: 'session-1',
          kind: 'session',
          pinned_at: '2026-08-06T11:00:00Z',
        }]}
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onArchiveItem={vi.fn()}
      />,
    )

    expect(screen.getByLabelText('Pinned')).toBeInTheDocument()
  })
})

describe('ProjectChatGroup archived mode', () => {
  it('requests archived rows and offers to restore them', () => {
    const onArchiveItem = vi.fn()
    mocks.tasks = [{
      id: 'archived-task',
      project_id: 'project-one',
      name: 'Archived task',
      status: TypesSpecTaskStatus.TaskStatusDone,
      status_updated_at: '2026-08-05T10:00:00Z',
    }]

    render(
      <ProjectChatGroup
        orgId="org-one"
        project={{ id: 'project-one', name: 'Empty project' }}
        collapsed={false}
        query=""
        activeItemId=""
        relativeTimeNow={Date.UTC(2026, 7, 6, 12, 0)}
        enabled
        participantIds={[]}
        organizationMembers={[]}
        archived
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
        onArchiveItem={onArchiveItem}
      />,
    )

    expect(mocks.sessionOptions.every((options) => options.archived === true)).toBe(true)
    expect(mocks.taskOptions.every((options) => options.archivedOnly === true)).toBe(true)
    // Archived rows must never keep polling in the background.
    expect(mocks.taskOptions.every((options) => options.refetchInterval === false)).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Unarchive task Archived task' }))
    expect(onArchiveItem).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: /^Archive task/ })).not.toBeInTheDocument()
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
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={vi.fn()}
        onOpenItemContextMenu={vi.fn()}
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
        participantIds={[]}
        organizationMembers={[]}
        archivingItemId={null}
        onToggle={vi.fn()}
        onOpenItem={onOpenItem}
        onOpenItemContextMenu={vi.fn()}
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
