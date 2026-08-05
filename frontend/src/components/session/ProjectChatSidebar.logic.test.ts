import { describe, expect, it } from 'vitest'
import type { TypesProject, TypesSessionSummary } from '../../api/api'
import type { SpecTask } from '../../services/specTaskService'
import {
  buildProjectChatGroups,
  compactRelativeTime,
  filterProjectChatGroups,
  getSidebarTaskStatus,
} from './ProjectChatSidebar.logic'

const projects: TypesProject[] = [
  { id: 'project-one', name: 'Project One' },
  { id: 'project-two', name: 'Project Two' },
]

describe('ProjectChatSidebar logic', () => {
  it('groups tasks by project, deduplicates their sessions, and puts direct chats in Default', () => {
    const tasks: SpecTask[] = [{
      id: 'task-one',
      project_id: 'project-one',
      name: 'Implement grouped sidebar',
      status: 'implementation',
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

    expect(groups.map((group) => group.name)).toEqual(['Default', 'Project One', 'Project Two'])
    expect(groups[0]?.items.map((item) => item.id)).toEqual(['direct-session', 'worker-session'])
    expect(groups[1]?.items.map((item) => item.id)).toEqual(['task-one'])
    expect(groups[2]?.items.map((item) => item.id)).toEqual(['project-session'])
  })

  it('uses AND matching across multiple search tokens', () => {
    const groups = buildProjectChatGroups(projects, [{
      id: 'task-one',
      project_id: 'project-one',
      name: 'Prevent unsafe image updates',
      status: 'done',
    }], [])

    expect(filterProjectChatGroups(groups, 'unsafe image')[0]?.items).toHaveLength(1)
    expect(filterProjectChatGroups(groups, 'unsafe missing')).toHaveLength(0)
  })

  it('maps workflow phases and formats compact relative times', () => {
    expect(getSidebarTaskStatus({ status: 'spec_generation' })?.label).toBe('Planning')
    expect(getSidebarTaskStatus({ status: 'implementation_review' })?.label).toBe('Review')
    expect(getSidebarTaskStatus({ status: 'done' })?.label).toBe('Completed')
    expect(compactRelativeTime('2026-08-05T11:55:00Z', Date.parse('2026-08-05T12:00:00Z'))).toBe('5m')
  })
})
