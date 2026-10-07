import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ProjectChatSidebarProjectFilter from './ProjectChatSidebarProjectFilter'
import { ALL_PROJECTS_FILTER, ALL_USERS_FILTER } from './ProjectChatSidebar.logic'
import type { SidebarMember } from './ProjectChatSidebar.logic'

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

const projects = [
  { id: 'project-b', name: 'Beta project' },
  { id: 'project-a', name: 'Alpha project' },
]

const members: SidebarMember[] = [
  { userId: 'user-me', user: { email: 'me@helix.local' }, online: true, isViewer: true },
  { userId: 'user-alice', user: { full_name: 'Alice Example' }, online: true },
]

const renderFilter = (
  overrides: Partial<Parameters<typeof ProjectChatSidebarProjectFilter>[0]> = {},
  onProjectChange = vi.fn(),
  onUserChange = vi.fn(),
) => ({
  onProjectChange,
  onUserChange,
  ...render(
    <ProjectChatSidebarProjectFilter
      projects={projects}
      selectedProjectId={ALL_PROJECTS_FILTER}
      archived={false}
      members={members}
      selectedUserId={ALL_USERS_FILTER}
      showUserFilter
      onChange={onProjectChange}
      onUserChange={onUserChange}
      {...overrides}
    />,
  ),
})

describe('ProjectChatSidebarProjectFilter', () => {
  it('lists projects alphabetically and then the user filter section', () => {
    renderFilter()

    fireEvent.click(screen.getByRole('button', { name: 'Filter tasks by project' }))

    expect(screen.getByRole('menuitem', { name: 'All projects' })).toBeInTheDocument()
    const items = screen.getAllByRole('menuitem').map((item) => item.textContent?.trim() || '')
    expect(items.slice(0, 4)).toEqual(['All projects', 'Alpha project', 'Beta project', 'Everyone'])
    // Member rows carry an initials avatar before the label.
    expect(items[4]).toContain('me@helix.local (you)')
    expect(items[5]).toContain('Alice Example')
    expect(screen.getByText('Filter by user')).toBeInTheDocument()
  })

  it('reports the picked project and user separately', () => {
    const { onProjectChange, onUserChange } = renderFilter()

    fireEvent.click(screen.getByRole('button', { name: 'Filter tasks by project' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Alpha project' }))
    expect(onProjectChange).toHaveBeenCalledWith('project-a')

    fireEvent.click(screen.getByRole('button', { name: 'Filter tasks by project' }))
    fireEvent.click(screen.getByRole('menuitem', { name: /Alice Example/ }))
    expect(onUserChange).toHaveBeenCalledWith('user-alice')

    fireEvent.click(screen.getByRole('button', { name: 'Filter tasks by project' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Everyone' }))
    expect(onUserChange).toHaveBeenCalledWith(ALL_USERS_FILTER)
  })

  it('marks the button label with the active user filter', () => {
    renderFilter({ selectedUserId: 'user-alice' })

    expect(screen.getByRole('button', { name: 'Filter tasks by project' }))
      .toHaveTextContent('All projects · Alice Example')
  })

  it('hides the user section when the view cannot use it', () => {
    renderFilter({ showUserFilter: false })

    fireEvent.click(screen.getByRole('button', { name: 'Filter tasks by project' }))

    expect(screen.queryByRole('menuitem', { name: 'Everyone' })).not.toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Alpha project' })).toBeInTheDocument()
  })

  it('keeps the user out of the label when the filter is not in effect', () => {
    renderFilter({ selectedUserId: 'user-alice', showUserFilter: false })

    expect(screen.getByRole('button', { name: 'Filter tasks by project' }))
      .toHaveTextContent('All projects')
  })
})
