import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ProjectChatSidebarOptions from './ProjectChatSidebarOptions'

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

describe('ProjectChatSidebarOptions', () => {
  it('changes sort modes and the visible thread count without closing the menu', () => {
    const onProjectSortOrderChange = vi.fn()
    const onThreadSortOrderChange = vi.fn()
    const onVisibleThreadCountChange = vi.fn()
    render(
      <ProjectChatSidebarOptions
        projectSortOrder="updated_at"
        threadSortOrder="updated_at"
        visibleThreadCount={6}
        onProjectSortOrderChange={onProjectSortOrderChange}
        onThreadSortOrderChange={onThreadSortOrderChange}
        onVisibleThreadCountChange={onVisibleThreadCountChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Sidebar options' }))
    fireEvent.click(screen.getByRole('menuitemradio', { name: 'Manual' }))
    fireEvent.click(screen.getAllByRole('menuitemradio', { name: 'Created at' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Increase visible thread count' }))

    expect(onProjectSortOrderChange).toHaveBeenCalledWith('manual')
    expect(onThreadSortOrderChange).toHaveBeenCalledWith('created_at')
    expect(onVisibleThreadCountChange).toHaveBeenCalledWith(7)
    expect(screen.getByText('Sort projects')).toBeVisible()
  })
})
