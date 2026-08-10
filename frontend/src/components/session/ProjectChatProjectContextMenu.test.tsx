import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ProjectChatProjectContextMenu from './ProjectChatProjectContextMenu'

describe('ProjectChatProjectContextMenu', () => {
  it('opens the project board and settings actions', () => {
    const project = { id: 'project-one', name: 'Project One' }
    const onClose = vi.fn()
    const onOpenBoard = vi.fn()
    const onOpenSettings = vi.fn()
    const { rerender } = render(
      <ProjectChatProjectContextMenu
        project={project}
        position={{ mouseX: 100, mouseY: 120 }}
        onClose={onClose}
        onOpenBoard={onOpenBoard}
        onOpenSettings={onOpenSettings}
      />,
    )

    fireEvent.click(screen.getByRole('menuitem', { name: 'Project board' }))
    expect(onClose).toHaveBeenCalledOnce()
    expect(onOpenBoard).toHaveBeenCalledWith(project)

    rerender(
      <ProjectChatProjectContextMenu
        project={project}
        position={{ mouseX: 100, mouseY: 120 }}
        onClose={onClose}
        onOpenBoard={onOpenBoard}
        onOpenSettings={onOpenSettings}
      />,
    )
    fireEvent.click(screen.getByRole('menuitem', { name: 'Project settings' }))
    expect(onClose).toHaveBeenCalledTimes(2)
    expect(onOpenSettings).toHaveBeenCalledWith(project)
  })
})
