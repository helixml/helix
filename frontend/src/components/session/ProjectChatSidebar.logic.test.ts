import { describe, expect, it } from 'vitest'
import { TypesAgentWorkState, TypesSpecTaskStatus } from '../../api/api'
import type { TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import {
  buildProjectChatGroups,
  collapsedGroupsStorageKey,
  compactRelativeTime,
  filterProjectChatGroups,
  getSidebarPullRequestIcon,
  getSidebarTaskStatus,
  isNewThreadShortcut,
  isTaskCompletedOrMerged,
  parseCollapsedGroupIds,
  serializeCollapsedGroupIds,
  shouldConfirmTaskArchive,
} from './ProjectChatSidebar.logic'

const projects: TypesProject[] = [
  { id: 'project-one', name: 'Project One' },
  { id: 'project-two', name: 'Project Two' },
]

describe('ProjectChatSidebar logic', () => {
  it('groups tasks and project-linked chats by project, and puts direct chats in None', () => {
    const tasks: SpecTask[] = [{
      id: 'task-one',
      project_id: 'project-one',
      name: 'Implement grouped sidebar',
      status: TypesSpecTaskStatus.TaskStatusImplementation,
      updated_at: '2026-08-05T10:00:00Z',
    }]
    const sessions: TypesSessionSummary[] = [
      {
        session_id: 'task-session',
        name: 'Task work session',
        updated: '2026-08-05T11:00:00Z',
        metadata: { project_id: 'project-one', spec_task_id: 'task-one' },
      },
      {
        session_id: 'project-session',
        name: 'Project exploration',
        updated: '2026-08-05T09:00:00Z',
        metadata: { project_id: 'project-two' },
      },
      {
        session_id: 'direct-session',
        name: 'Direct model chat',
        updated: '2026-08-05T12:00:00Z',
      },
      {
        session_id: 'worker-session',
        name: 'Agent chat',
        updated: '2026-08-05T08:00:00Z',
        metadata: { project_id: 'project-one', org_worker_id: 'worker-one' },
      },
    ]

    const groups = buildProjectChatGroups(projects, tasks, sessions)

    expect(groups.map((group) => group.name)).toEqual(['None', 'Project One', 'Project Two'])
    expect(groups[0]?.items.map((item) => item.id)).toEqual(['direct-session'])
    expect(groups[1]?.items.map((item) => item.id)).toEqual(['task-one', 'worker-session'])
    expect(groups[2]?.items.map((item) => item.id)).toEqual(['project-session'])
  })

  it('uses AND matching across multiple search tokens', () => {
    const groups = buildProjectChatGroups(projects, [{
      id: 'task-one',
      project_id: 'project-one',
      name: 'Prevent unsafe image updates',
      status: TypesSpecTaskStatus.TaskStatusDone,
    }], [])

    expect(filterProjectChatGroups(groups, 'unsafe image')[0]?.items).toHaveLength(1)
    expect(filterProjectChatGroups(groups, 'unsafe missing')).toHaveLength(0)
  })

  it('maps workflow phases and formats compact relative times', () => {
    expect(getSidebarTaskStatus({ status: TypesSpecTaskStatus.TaskStatusSpecGeneration })?.label).toBe('Planning')
    expect(getSidebarTaskStatus({ status: TypesSpecTaskStatus.TaskStatusImplementationReview })?.label).toBe('Review')
    expect(getSidebarTaskStatus({ status: TypesSpecTaskStatus.TaskStatusImplementation })).toMatchObject({
      label: 'Implementation',
      color: '#34d399',
    })
    expect(getSidebarTaskStatus({ status: TypesSpecTaskStatus.TaskStatusDone })).toMatchObject({
      label: 'Completed',
      color: '#a78bfa',
    })
    expect(compactRelativeTime('2026-08-05T11:59:01Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('now')
    expect(compactRelativeTime('2026-08-05T11:59:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('1m')
    expect(compactRelativeTime('2026-08-05T11:55:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('5m')
    expect(compactRelativeTime('2026-08-05T06:00:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('6h')
  })

  it('shows runtime state ahead of the workflow phase', () => {
    expect(getSidebarTaskStatus({
      status: TypesSpecTaskStatus.TaskStatusImplementation,
      sandbox_state: 'running',
      agent_work_state: TypesAgentWorkState.AgentWorkStateIdle,
    })).toEqual({ label: 'Idle', color: '#fbbf24' })

    expect(getSidebarTaskStatus({
      status: TypesSpecTaskStatus.TaskStatusDone,
      sandbox_state: 'running',
      agent_work_state: TypesAgentWorkState.AgentWorkStateDone,
    })).toEqual({ label: 'Idle', color: '#fbbf24' })

    expect(getSidebarTaskStatus({
      status: TypesSpecTaskStatus.TaskStatusSpecGeneration,
      sandbox_state: 'running',
      agent_work_state: TypesAgentWorkState.AgentWorkStateWorking,
    })?.label).toBe('Planning')
  })

  it('greys offline tasks and explains their state', () => {
    expect(getSidebarTaskStatus({
      status: TypesSpecTaskStatus.TaskStatusDone,
      sandbox_state: 'absent',
      agent_work_state: TypesAgentWorkState.AgentWorkStateIdle,
    })).toEqual({
      label: 'Completed',
      color: '#a1a1aa',
      tooltip: 'Sandbox and agent are offline',
    })
    expect(getSidebarTaskStatus({ sandbox_state: 'absent' })?.label).toBe('Offline')
  })

  it('colors pull request icons independently from sandbox state', () => {
    const offlineTask = {
      status: TypesSpecTaskStatus.TaskStatusPullRequest,
      sandbox_state: 'absent',
    }

    expect(getSidebarPullRequestIcon({
      ...offlineTask,
      repo_pull_requests: [{ pr_state: 'open', pr_url: 'https://github.com/helixml/helix/pull/1' }],
    })).toEqual({
      color: '#10b981',
      tooltip: 'Pull request is open',
      url: 'https://github.com/helixml/helix/pull/1',
    })
    expect(getSidebarPullRequestIcon({
      ...offlineTask,
      repo_pull_requests: [{ pr_state: 'closed' }],
    })).toMatchObject({ color: '#ef4444', tooltip: 'Pull request is closed' })
    expect(getSidebarPullRequestIcon({
      ...offlineTask,
      repo_pull_requests: [{ pr_state: 'merged' }],
    })).toMatchObject({ color: '#8b5cf6', tooltip: 'Pull request is merged' })
    expect(getSidebarPullRequestIcon({
      ...offlineTask,
      merged_to_main: true,
    })).toMatchObject({ color: '#8b5cf6', tooltip: 'Pull request is merged' })
    expect(getSidebarPullRequestIcon({
      ...offlineTask,
      repo_pull_requests: [{ pr_state: 'merged' }, { pr_state: 'open' }],
    })).toMatchObject({ color: '#10b981', tooltip: 'Pull request is open' })
    expect(getSidebarPullRequestIcon(offlineTask)).toEqual({
      color: '#a1a1aa',
      tooltip: 'No pull request yet',
    })
  })

  it('skips archive confirmation for stopped completed or merged tasks', () => {
    const completedTask: SpecTask = {
      id: 'completed-task',
      status: TypesSpecTaskStatus.TaskStatusDone,
      sandbox_state: 'absent',
    }
    const mergedTask: SpecTask = {
      id: 'merged-task',
      status: TypesSpecTaskStatus.TaskStatusPullRequest,
      merged_to_main: true,
      sandbox_state: 'absent',
    }

    expect(isTaskCompletedOrMerged(completedTask)).toBe(true)
    expect(isTaskCompletedOrMerged(mergedTask)).toBe(true)
    expect(shouldConfirmTaskArchive({
      id: completedTask.id!,
      kind: 'spec-task',
      title: 'Completed task',
      task: completedTask,
    })).toBe(false)
    expect(shouldConfirmTaskArchive({
      id: mergedTask.id!,
      kind: 'spec-task',
      title: 'Merged task',
      task: mergedTask,
    })).toBe(false)
  })

  it('keeps confirmation when a task may still stop an agent', () => {
    expect(shouldConfirmTaskArchive({
      id: 'running-task',
      kind: 'spec-task',
      title: 'Running task',
      task: {
        status: TypesSpecTaskStatus.TaskStatusDone,
        sandbox_state: 'running',
      },
    })).toBe(true)
    expect(shouldConfirmTaskArchive({
      id: 'unknown-task',
      kind: 'spec-task',
      title: 'Unknown task state',
    })).toBe(true)
  })

  it('archives org-agent chats without destructive confirmation', () => {
    const item = {
      id: 'org-agent-chat',
      kind: 'session' as const,
      title: 'Org agent chat',
      session: {
        session_id: 'org-agent-chat',
        app_id: 'app-org-agent',
        metadata: { agent_type: 'zed_external' },
      },
    }

    expect(shouldConfirmTaskArchive(item, new Set(['app-org-agent']))).toBe(false)
    expect(shouldConfirmTaskArchive({
      ...item,
      session: {
        ...item.session,
        app_id: 'app-unrelated',
        metadata: { ...item.session.metadata, org_worker_id: 'worker-one' },
      },
    })).toBe(false)
  })

  it('keeps destructive confirmation for ordinary external-agent chats', () => {
    expect(shouldConfirmTaskArchive({
      id: 'external-chat',
      kind: 'session',
      title: 'External chat',
      session: {
        session_id: 'external-chat',
        app_id: 'app-ordinary',
        metadata: { agent_type: 'zed_external' },
      },
    }, new Set(['app-org-agent']))).toBe(true)
  })

  it('stores collapsed groups independently for each organization', () => {
    const collapsed = new Set(['project-two', 'default'])

    expect(collapsedGroupsStorageKey('org-one')).not.toBe(collapsedGroupsStorageKey('org-two'))
    expect([...parseCollapsedGroupIds(serializeCollapsedGroupIds(collapsed))].sort()).toEqual([
      'default',
      'project-two',
    ])
    expect(parseCollapsedGroupIds('invalid json')).toEqual(new Set())
  })

  it('recognizes the new-thread shortcut on macOS and other platforms', () => {
    expect(isNewThreadShortcut({ key: 'n', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false })).toBe(true)
    expect(isNewThreadShortcut({ key: 'N', metaKey: false, ctrlKey: true, altKey: false, shiftKey: false })).toBe(true)
    expect(isNewThreadShortcut({ key: 'n', metaKey: false, ctrlKey: false, altKey: false, shiftKey: false })).toBe(false)
    expect(isNewThreadShortcut({ key: 'n', metaKey: true, ctrlKey: false, altKey: false, shiftKey: true })).toBe(false)
  })
})
