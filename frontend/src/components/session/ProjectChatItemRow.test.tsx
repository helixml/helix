import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TYPOGRAPHY } from '../../styles/typography'
import ProjectChatItemRow from './ProjectChatItemRow'
import type { SidebarItem } from './ProjectChatSidebar.logic'

const mocks = vi.hoisted(() => ({
  isPhone: false,
  repositories: [] as Array<{ external_type?: string; external_url?: string }>,
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../../hooks/useIsPhone', () => ({
  default: () => mocks.isPhone,
}))

vi.mock('../../hooks/useApps', () => ({
  default: () => ({ apps: [] }),
}))

vi.mock('../../services/projectService', () => ({
  useGetProjectRepositories: () => ({ data: mocks.repositories }),
}))

afterEach(() => {
  vi.clearAllMocks()
  mocks.isPhone = false
  mocks.repositories = []
})

const NOW = Date.parse('2026-09-07T12:00:00Z')

const renderRow = (item: SidebarItem) => render(
  <ProjectChatItemRow
    item={item}
    active={false}
    relativeTimeNow={NOW}
    archivingItemId={null}
    organizationMembers={[]}
    projectName={item.projectName}
    onOpenItem={vi.fn()}
    onOpenItemContextMenu={vi.fn()}
    onArchiveItem={vi.fn()}
  />,
)

const quietTask: SidebarItem = {
  id: 'task-1',
  kind: 'spec-task',
  title: 'Fix the installer',
  updatedAt: '2026-09-05T12:00:00Z',
  projectId: 'prj-1',
  projectName: 'keel',
  task: {
    id: 'task-1',
    status: 'done',
    sandbox_state: 'absent',
    branch_name: 'fix/installer',
    code_agent_config: { runtime: 'qwen_code' },
  } as any,
}

describe('ProjectChatItemRow', () => {
  it('stacks the project name above the title only for cross-project rows', () => {
    const { container } = renderRow(quietTask)
    const projectName = screen.getByText('keel')
    const title = screen.getByText('Fix the installer')
    expect(projectName).toBeInTheDocument()
    expect(title).toBeInTheDocument()
    expect(projectName).toHaveStyle({
      fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
      fontWeight: '500',
    })
    expect(title).toHaveStyle({
      fontSize: TYPOGRAPHY.sidebar.primaryFontSize,
      fontWeight: '500',
    })
    expect(container.querySelector('.project-chat-item')).toHaveStyle({ minHeight: '78px' })
    expect(screen.getByTestId('sidebar-item-metadata')).toHaveTextContent('fix/installer')
    expect(screen.getByTestId('sidebar-item-harness')).toHaveStyle({ marginLeft: 'auto' })
    expect(screen.getByRole('img', { name: 'Qwen Code' })).toBeInTheDocument()

    renderRow({ ...quietTask, id: 'task-2', projectName: undefined })
    expect(screen.getAllByText('keel')).toHaveLength(1)
  })

  it('shows the relative time for quiet tasks and the status label for active ones', () => {
    renderRow(quietTask)
    expect(screen.getByText('2d')).toBeInTheDocument()
    expect(screen.queryByText('Implementation')).not.toBeInTheDocument()

    renderRow({
      ...quietTask,
      id: 'task-3',
      task: { id: 'task-3', status: 'implementation', sandbox_state: 'running', agent_work_state: 'working' } as any,
    })
    expect(screen.getByText('Implementation')).toBeInTheDocument()
  })

  it('explains why a planning task has no branch yet', async () => {
    renderRow({
      ...quietTask,
      id: 'task-planning',
      task: {
        ...quietTask.task,
        id: 'task-planning',
        status: 'spec_generation',
        branch_name: undefined,
        base_branch: undefined,
      } as any,
    })

    const branch = screen.getByTestId('sidebar-item-branch')
    expect(branch).toHaveAttribute('data-branch-state', 'unavailable')
    expect(branch).toHaveTextContent('n/a')

    fireEvent.mouseOver(branch)
    expect(await screen.findByText('Task is in planning mode; no branch yet.')).toBeInTheDocument()
    expect(screen.getAllByRole('tooltip')).toHaveLength(1)
  })

  it('renders the GitHub owner avatar and falls back to the folder glyph on load failure', () => {
    mocks.repositories = [{ external_type: 'github', external_url: 'https://github.com/keel-hq/keel' }]
    const { container } = renderRow(quietTask)
    const img = container.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://github.com/keel-hq.png?size=48')

    fireEvent.error(img!)
    expect(container.querySelector('img')).toBeNull()
  })

  it('keeps time and status label visible on a phone', () => {
    mocks.isPhone = true
    renderRow(quietTask)
    // Both are static row content on a phone — no hover exists to reveal them.
    expect(screen.getByText('2d')).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
  })
})
