import { describe, expect, it } from 'vitest'
import { TypesAgentWorkState, TypesCodeAgentRuntime, TypesSpecTaskStatus } from '../../api/api'
import type { TypesOrganizationMembership, TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import {
  buildProjectChatGroups,
  clampVisibleThreadCount,
  collapsedGroupsStorageKey,
  compactRelativeTime,
  DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES,
  filterProjectChatGroups,
  filterSidebarMembers,
  getSidebarPullRequestIcon,
  getSidebarMemberResults,
  getSidebarTaskStatus,
  getChatShortcutNumber,
  isChatShortcutModifier,
  isNewThreadShortcut,
  isTaskCompletedOrMerged,
  parseCollapsedGroupIds,
  parseSidebarPreferences,
  parseSidebarParticipantIds,
  reorderProjectIds,
  serializeCollapsedGroupIds,
  serializeSidebarPreferences,
  serializeSidebarParticipantIds,
  shouldConfirmArchive,
  sidebarPreferencesStorageKey,
  sidebarPeopleFilterStorageKey,
  sortSidebarProjects,
  specTaskSortKey,
} from './ProjectChatSidebar.logic'

const projects: TypesProject[] = [
  { id: 'project-one', name: 'Project One' },
  { id: 'project-two', name: 'Project Two' },
]

describe('ProjectChatSidebar logic', () => {
  it('uses Command on macOS and Control elsewhere for chat shortcuts', () => {
    expect(isChatShortcutModifier({ metaKey: true, ctrlKey: false }, true)).toBe(true)
    expect(isChatShortcutModifier({ metaKey: false, ctrlKey: true }, true)).toBe(false)
    expect(isChatShortcutModifier({ metaKey: false, ctrlKey: true }, false)).toBe(true)
    expect(isChatShortcutModifier({ metaKey: true, ctrlKey: false }, false)).toBe(false)
  })

  it('accepts only unmodified chat shortcut numbers 1 through 9', () => {
    expect(getChatShortcutNumber({ key: '1', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false }, true)).toBe(1)
    expect(getChatShortcutNumber({ key: '9', metaKey: false, ctrlKey: true, altKey: false, shiftKey: false }, false)).toBe(9)
    expect(getChatShortcutNumber({ key: '0', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false }, true)).toBeNull()
    expect(getChatShortcutNumber({ key: '2', metaKey: true, ctrlKey: false, altKey: false, shiftKey: true }, true)).toBeNull()
  })

  it('groups tasks and project-linked chats by project, and puts direct chats in None', () => {
    const tasks: SpecTask[] = [{
      id: 'task-one',
      project_id: 'project-one',
      name: 'Implement grouped sidebar',
      status: TypesSpecTaskStatus.TaskStatusImplementation,
      status_updated_at: '2026-08-05T10:00:00Z',
    }]
    const sessions: TypesSessionSummary[] = [
      {
        session_id: 'task-session',
        name: 'Task work session',
        model_name: 'claude-opus-4-6',
        updated: '2026-08-05T11:00:00Z',
        metadata: {
          project_id: 'project-one',
          spec_task_id: 'task-one',
          code_agent_runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
        },
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
    expect(groups[1]?.items[0]?.session).toEqual(expect.objectContaining({
      session_id: 'task-session',
      model_name: 'claude-opus-4-6',
    }))
    expect(groups[2]?.items.map((item) => item.id)).toEqual(['project-session'])
  })

  // A group merges two independently limited server lists, so the client sort
  // key has to be the same last-message value used for server pagination.
  it('orders tasks by the server-backed last-message timestamp', () => {
    const task: SpecTask = {
      id: 'task-one',
      project_id: 'project-one',
      name: 'Recently chatted, long since moved',
      status: TypesSpecTaskStatus.TaskStatusImplementation,
      created_at: '2026-08-01T00:00:00Z',
      status_updated_at: '2026-08-02T00:00:00Z',
      updated_at: '2026-08-09T00:00:00Z',
      last_message_at: '2026-08-08T00:00:00Z',
      session_updated_at: '2026-08-09T00:00:00Z',
    }

    expect(specTaskSortKey(task)).toBe('2026-08-08T00:00:00Z')
    expect(buildProjectChatGroups(projects, [task], [])[0]?.items[0]?.updatedAt)
      .toBe('2026-08-08T00:00:00Z')
    expect(specTaskSortKey({ id: 'x', created_at: '2026-08-01T00:00:00Z' }))
      .toBe('2026-08-01T00:00:00Z')
    expect(specTaskSortKey(task, 'created_at')).toBe('2026-08-01T00:00:00Z')
  })

  it('sorts threads by their requested server-backed timestamp', () => {
    const tasks: SpecTask[] = [
      {
        id: 'older-created-recently-updated',
        project_id: 'project-one',
        name: 'Older created',
        created_at: '2026-08-01T00:00:00Z',
        last_message_at: '2026-08-06T00:00:00Z',
        status_updated_at: '2026-08-06T00:00:00Z',
      },
      {
        id: 'newer-created',
        project_id: 'project-one',
        name: 'Newer created',
        created_at: '2026-08-05T00:00:00Z',
        last_message_at: '2026-08-05T00:00:00Z',
        status_updated_at: '2026-08-05T00:00:00Z',
      },
    ]

    expect(buildProjectChatGroups(projects, tasks, [], 'updated_at')[0]?.items.map((item) => item.id))
      .toEqual(['older-created-recently-updated', 'newer-created'])
    expect(buildProjectChatGroups(projects, tasks, [], 'created_at')[0]?.items.map((item) => item.id))
      .toEqual(['newer-created', 'older-created-recently-updated'])
  })

  it('does not treat session metadata updates as conversation messages', () => {
    const sessions: TypesSessionSummary[] = [
      {
        session_id: 'recent-metadata',
        created: '2026-08-01T00:00:00Z',
        updated: '2026-08-09T00:00:00Z',
        last_message_at: '2026-08-05T00:00:00Z',
      },
      {
        session_id: 'recent-message',
        created: '2026-08-02T00:00:00Z',
        updated: '2026-08-06T00:00:00Z',
        last_message_at: '2026-08-08T00:00:00Z',
      },
    ]

    expect(buildProjectChatGroups([], [], sessions)[0]?.items.map((item) => item.id))
      .toEqual(['recent-message', 'recent-metadata'])
  })

  it('orders pinned chats before unpinned chats with newest pins first', () => {
    const sessions: TypesSessionSummary[] = [
      { session_id: 'newest-chat', created: '2026-08-08T00:00:00Z' },
      { session_id: 'older-pin', created: '2026-08-01T00:00:00Z' },
      { session_id: 'newer-pin', created: '2026-08-02T00:00:00Z' },
    ]
    const pins = new Map([
      ['session:older-pin', '2026-08-06T00:00:00Z'],
      ['session:newer-pin', '2026-08-07T00:00:00Z'],
    ])

    expect(buildProjectChatGroups([], [], sessions, 'updated_at', pins)[0]?.items.map((item) => item.id))
      .toEqual(['newer-pin', 'older-pin', 'newest-chat'])
  })

  it('parses and clamps org-scoped local preferences', () => {
    const parsed = parseSidebarPreferences(JSON.stringify({
      projectSortOrder: 'manual',
      threadSortOrder: 'created_at',
      visibleThreadCount: 100,
      manualProjectOrder: ['project-two', 'project-two', '', 12, 'project-one'],
    }))

    expect(sidebarPreferencesStorageKey('org-one')).toBe('helix:project-chat-sidebar:preferences:org-one')
    expect(parsed).toEqual({
      projectSortOrder: 'manual',
      threadSortOrder: 'created_at',
      visibleThreadCount: 15,
      manualProjectOrder: ['project-two', 'project-one'],
    })
    expect(parseSidebarPreferences('{bad json')).toEqual(DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES)
    expect(parseSidebarPreferences(serializeSidebarPreferences(parsed))).toEqual(parsed)
    expect(clampVisibleThreadCount(-2)).toBe(1)
  })

  it('persists selected people per user, organization, and project and searches every token', () => {
    const members: TypesOrganizationMembership[] = [
      { user_id: 'alice', user: { full_name: 'Alice Example', email: 'alice@example.com' } },
      { user_id: 'bob', user: { full_name: 'Bob Builder', email: 'bob@work.test' } },
      { user_id: 'invite', user: undefined },
    ]

    expect(sidebarPeopleFilterStorageKey('user-one', 'org-one', 'project-one'))
      .toBe('helix:project-chat-sidebar:people:user-one:org-one:project-one')
    expect(parseSidebarParticipantIds('["bob","bob","",12]')).toEqual(['bob'])
    expect(parseSidebarParticipantIds('[]')).toEqual([])
    expect(parseSidebarParticipantIds(null)).toBeNull()
    expect(parseSidebarParticipantIds('{bad json')).toBeNull()
    expect(serializeSidebarParticipantIds(['bob', 'alice', 'bob'])).toBe('["bob","alice"]')
    expect(filterSidebarMembers(members, 'alice example')).toEqual([members[0]])
    expect(filterSidebarMembers(members, 'bob work')).toEqual([members[1]])
    expect(filterSidebarMembers(members, 'alice missing')).toEqual([])
  })

  it('shows at most ten people before searching, with the current and selected users first', () => {
    const members: TypesOrganizationMembership[] = Array.from({ length: 12 }, (_, index) => ({
      user_id: `user-${index}`,
      user: { full_name: `Member ${index}`, email: `member-${index}@example.com` },
    }))

    const initial = getSidebarMemberResults(members, '', 'user-11', ['user-10'])
    expect(initial.total).toBe(12)
    expect(initial.members).toHaveLength(10)
    expect(initial.members.slice(0, 2).map((member) => member.user_id)).toEqual(['user-11', 'user-10'])

    const searched = getSidebarMemberResults(members, 'member-3 example', 'user-11', ['user-10'])
    expect(searched.total).toBe(1)
    expect(searched.members[0]?.user_id).toBe('user-3')
  })

  it('sorts projects by activity, creation, and persisted manual order', () => {
    const sortableProjects: TypesProject[] = [
      {
        id: 'project-one',
        name: 'One',
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-03T00:00:00Z',
        last_activity_at: '2026-08-06T00:00:00Z',
      },
      {
        id: 'project-two',
        name: 'Two',
        created_at: '2026-08-05T00:00:00Z',
        updated_at: '2026-08-05T00:00:00Z',
        last_activity_at: '2026-08-05T00:00:00Z',
      },
    ]

    expect(sortSidebarProjects(sortableProjects, DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES)
      .map((project) => project.id)).toEqual(['project-one', 'project-two'])
    expect(sortSidebarProjects(sortableProjects, {
      ...DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES,
      projectSortOrder: 'created_at',
    }).map((project) => project.id)).toEqual(['project-two', 'project-one'])
    expect(sortSidebarProjects(sortableProjects, {
      ...DEFAULT_PROJECT_CHAT_SIDEBAR_PREFERENCES,
      projectSortOrder: 'manual',
      manualProjectOrder: ['project-two'],
    }).map((project) => project.id)).toEqual(['project-two', 'project-one'])
    expect(reorderProjectIds(['project-one', 'project-two'], 'project-one', 'project-two'))
      .toEqual(['project-two', 'project-one'])
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
    expect(shouldConfirmArchive({
      id: completedTask.id!,
      kind: 'spec-task',
      title: 'Completed task',
      task: completedTask,
    })).toBe(false)
    expect(shouldConfirmArchive({
      id: mergedTask.id!,
      kind: 'spec-task',
      title: 'Merged task',
      task: mergedTask,
    })).toBe(false)
  })

  it('keeps confirmation when a task may still stop an agent', () => {
    expect(shouldConfirmArchive({
      id: 'running-task',
      kind: 'spec-task',
      title: 'Running task',
      task: {
        status: TypesSpecTaskStatus.TaskStatusDone,
        sandbox_state: 'running',
      },
    })).toBe(true)
    expect(shouldConfirmArchive({
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

    expect(shouldConfirmArchive(item, new Set(['app-org-agent']))).toBe(false)
    expect(shouldConfirmArchive({
      ...item,
      session: {
        ...item.session,
        app_id: 'app-unrelated',
        metadata: { ...item.session.metadata, org_worker_id: 'worker-one' },
      },
    })).toBe(false)
  })

  it('keeps destructive confirmation for ordinary external-agent chats', () => {
    expect(shouldConfirmArchive({
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

  it('archives a plain model chat without warning about an agent it does not have', () => {
    expect(shouldConfirmArchive({
      id: 'plain-chat',
      kind: 'session',
      title: 'Plain chat',
      session: { session_id: 'plain-chat' },
    })).toBe(false)
  })

  it('never confirms when restoring from the archived view', () => {
    expect(shouldConfirmArchive({
      id: 'running-task',
      kind: 'spec-task',
      title: 'Running task',
      task: {
        status: TypesSpecTaskStatus.TaskStatusImplementation,
        sandbox_state: 'running',
      },
    }, new Set(), true)).toBe(false)
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
    expect(isNewThreadShortcut({ key: 'o', metaKey: true, ctrlKey: false, altKey: false, shiftKey: true })).toBe(true)
    expect(isNewThreadShortcut({ key: 'O', metaKey: false, ctrlKey: true, altKey: false, shiftKey: true })).toBe(true)
    expect(isNewThreadShortcut({ key: 'o', metaKey: false, ctrlKey: false, altKey: false, shiftKey: true })).toBe(false)
    expect(isNewThreadShortcut({ key: 'o', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false })).toBe(false)
  })

  // Cmd/Ctrl+N is reserved by the browser for "new window" and cannot be
  // preventDefault'ed, so binding it would open a window AND navigate.
  it('leaves the browser-reserved new-window chord alone', () => {
    expect(isNewThreadShortcut({ key: 'n', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false })).toBe(false)
    expect(isNewThreadShortcut({ key: 'n', metaKey: false, ctrlKey: true, altKey: false, shiftKey: false })).toBe(false)
  })
})
