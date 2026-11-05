import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TYPOGRAPHY } from '../../styles/typography'
import ProjectChatItemRow from './ProjectChatItemRow'
import type { SidebarItem } from './ProjectChatSidebar.logic'

const mocks = vi.hoisted(() => ({
  isPhone: false,
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

afterEach(() => {
  vi.clearAllMocks()
  mocks.isPhone = false
})

const NOW = Date.parse('2026-09-07T12:00:00Z')

const renderRow = (item: SidebarItem, active = false) => render(
  <ProjectChatItemRow
    item={item}
    active={active}
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
  it('subtly highlights the currently selected task', () => {
    const { container } = renderRow(quietTask, true)
    const row = container.querySelector('.project-chat-item')

    expect(row).toHaveAttribute('aria-current', 'page')
    expect(row).toHaveStyle({
      backgroundColor: 'rgba(241, 243, 247, 0.11)',
      boxShadow: 'inset 0 0 0 1px rgba(255, 255, 255, 0.08)',
    })
  })

  it('keeps cross-project rows on one line with the project only in the tooltip', () => {
    const { container } = renderRow(quietTask)
    expect(screen.getByText('Fix the installer')).toHaveStyle({
      fontSize: TYPOGRAPHY.sidebar.primaryFontSize,
      fontWeight: '500',
    })
    expect(screen.getByText('2d')).toBeInTheDocument()
    expect(screen.queryByText('keel')).not.toBeInTheDocument()
    expect(screen.queryByTestId('sidebar-item-metadata')).not.toBeInTheDocument()
    expect(container.querySelector('.project-chat-item')).toHaveStyle({ height: '32px' })
  })

  it('explains why a planning task has no branch yet', async () => {
    mocks.isPhone = true
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

  it('keeps time and status label visible on a phone', () => {
    mocks.isPhone = true
    renderRow(quietTask)
    // Both are static row content on a phone — no hover exists to reveal them.
    expect(screen.getByText('2d')).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
  })
})
