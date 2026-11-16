import { render, screen } from '@testing-library/react'
import createRouter from 'router5'
import { describe, expect, it, vi } from 'vitest'

import EmbedProjectPage, { openInHelixPath } from './EmbedProjectPage'

// A real router5 router with the real route's path, so the href is what buildPath
// actually produces — including its throw on characters like parentheses.
const realRouter = createRouter([
  { name: 'org_project-task-detail', path: '/orgs/:org_id/projects/:id/tasks/:taskId' },
])
const buildPath = (name: string, params: Record<string, string>) => realRouter.buildPath(name, params)

const mocks = vi.hoisted(() => ({
  query: {} as Record<string, string>,
  project: undefined as unknown,
}))

vi.mock('react-router5', () => ({
  useRoute: () => ({
    route: { params: { projectId: 'prj_y' } },
    router: { buildPath: (name: string, params: Record<string, string>) => buildPath(name, params) },
  }),
}))

vi.mock('../router', () => ({
  get initialQueryParams() {
    return mocks.query
  },
}))

vi.mock('../services', () => ({
  useGetProject: () => ({ data: mocks.project, isLoading: false }),
  useGetProjectExploratorySession: () => ({ data: undefined }),
}))

vi.mock('../components/session/AgentChat', () => ({ default: () => <div>agent chat</div> }))
vi.mock('../components/external-agent/ExternalAgentDesktopViewer', () => ({ default: () => <div>desktop</div> }))
vi.mock('../components/tasks/SpecTaskKanbanBoard', () => ({ default: () => <div>board</div> }))
vi.mock('../components/tasks/SpecTaskDetailContent', () => ({ default: () => <div>task workspace</div> }))
vi.mock('../components/tasks/NewSpecTaskForm', () => ({ default: () => <div>new task</div> }))

describe('EmbedProjectPage — Open in Helix', () => {
  it("links a board task to its full page in the project's org", () => {
    mocks.query = { view: 'board', task: 'spt_z' }
    mocks.project = { id: 'prj_y', name: 'Bot project', organization_id: 'org_x' }
    render(<EmbedProjectPage />)

    const link = screen.getByRole('link', { name: /Open in Helix/ })
    expect(link.getAttribute('href')).toBe('/orgs/org_x/projects/prj_y/tasks/spt_z')
    expect(link.getAttribute('target')).toBe('_blank')
  })

  it('has no link for a project outside an org', () => {
    mocks.query = { view: 'board', task: 'spt_z' }
    mocks.project = { id: 'prj_y', name: 'Bot project' }
    render(<EmbedProjectPage />)

    expect(screen.getByText('task workspace')).toBeTruthy()
    expect(screen.queryByRole('link', { name: /Open in Helix/ })).toBeNull()
  })

  // ?task= is host-controlled; router5 throws on parentheses, which used to crash the
  // whole embed to a white iframe.
  it('still renders, without the link, for a malformed ?task=', () => {
    mocks.query = { view: 'board', task: 'foo(bar)' }
    mocks.project = { id: 'prj_y', name: 'Bot project', organization_id: 'org_x' }
    render(<EmbedProjectPage />)

    expect(screen.getByText('task workspace')).toBeTruthy()
    expect(screen.queryByRole('link', { name: /Open in Helix/ })).toBeNull()
  })
})

describe('openInHelixPath', () => {
  it('refuses ids router5 would throw on, or that are not id-shaped', () => {
    expect(() => buildPath('org_project-task-detail', { org_id: 'o', id: 'p', taskId: 'a(b)' })).toThrow()
    for (const bad of ['a(b)', '../x', 'a/b', 'a?b', '']) {
      expect(openInHelixPath(buildPath, 'org_x', 'prj_y', bad)).toBeUndefined()
    }
    expect(openInHelixPath(buildPath, 'org_x', 'prj_y', 'spt_z')).toBe('/orgs/org_x/projects/prj_y/tasks/spt_z')
  })
})
