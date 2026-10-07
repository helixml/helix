import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ProjectChatSidebarUserFilter from './ProjectChatSidebarUserFilter'
import { ALL_USERS_FILTER } from './ProjectChatSidebar.logic'
import type { SidebarMember } from './ProjectChatSidebar.logic'

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

const members: SidebarMember[] = [
  { userId: 'user-me', user: { email: 'me@helix.local' }, online: true, isViewer: true },
  { userId: 'user-alice', user: { full_name: 'Alice Example' }, online: true },
]

const renderFilter = (selectedUserId = ALL_USERS_FILTER, onChange = vi.fn()) => ({
  onChange,
  ...render(
    <ProjectChatSidebarUserFilter
      members={members}
      selectedUserId={selectedUserId}
      onChange={onChange}
    />,
  ),
})

describe('ProjectChatSidebarUserFilter', () => {
  it('labels the control after the selected member', () => {
    renderFilter('user-alice')

    expect(screen.getByRole('button', { name: 'Filter chats by user' }))
      .toHaveTextContent('Alice Example')
  })

  it('lists everyone and each member in the picker', () => {
    renderFilter()

    fireEvent.click(screen.getByRole('button', { name: 'Filter chats by user' }))

    expect(screen.getByRole('menuitem', { name: 'Everyone' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Alice Example/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /me@helix\.local \(you\)/ })).toBeInTheDocument()
  })

  it('reports the picked member and resets to everyone', () => {
    const onChange = vi.fn()
    renderFilter(ALL_USERS_FILTER, onChange)

    fireEvent.click(screen.getByRole('button', { name: 'Filter chats by user' }))
    fireEvent.click(screen.getByRole('menuitem', { name: /Alice Example/ }))
    expect(onChange).toHaveBeenCalledWith('user-alice')

    fireEvent.click(screen.getByRole('button', { name: 'Filter chats by user' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Everyone' }))
    expect(onChange).toHaveBeenCalledWith(ALL_USERS_FILTER)
  })
})
