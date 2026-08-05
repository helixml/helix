import { describe, expect, it } from 'vitest'
import { TypesSpecTaskStatus } from '../../api/api'
import type { TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import {
  buildProjectChatGroups,
  collapsedGroupsStorageKey,
  compactRelativeTime,
  filterProjectChatGroups,
  getSidebarTaskStatus,
  isNewThreadShortcut,
  parseCollapsedGroupIds,
  serializeCollapsedGroupIds,
} from './ProjectChatSidebar.logic'

const projects: TypesProject[] = [
  { id: 'project-one', name: 'Project One' },
  { id: 'project-two', name: 'Project Two' },
]

describe('ProjectChatSidebar logic', () => {
  it('groups tasks by project, deduplicates their sessions, and puts direct chats in None', () => {
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
    expect(groups[0]?.items.map((item) => item.id)).toEqual(['direct-session', 'worker-session'])
    expect(groups[1]?.items.map((item) => item.id)).toEqual(['task-one'])
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
    expect(getSidebarTaskStatus({ status: TypesSpecTaskStatus.TaskStatusDone })?.label).toBe('Completed')
    expect(compactRelativeTime('2026-08-05T11:59:01Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('now')
    expect(compactRelativeTime('2026-08-05T11:59:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('1m')
    expect(compactRelativeTime('2026-08-05T11:55:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('5m')
    expect(compactRelativeTime('2026-08-05T06:00:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('6h')
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
