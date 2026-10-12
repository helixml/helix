import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import NewChatProjectDialog, { buildNewChatRows } from './NewChatProjectDialog'

vi.mock('@mui/material/useMediaQuery', () => ({ default: () => false }))

const projects = [
  { id: 'prj_a', name: 'home stuff', github_repo_url: 'https://github.com/helixml/helix' },
  { id: 'prj_b', name: 'webhookrelay', description: 'Relay for webhooks' },
  { id: 'prj_c' },
]

describe('buildNewChatRows', () => {
  it('offers projects only', () => {
    const rows = buildNewChatRows(projects, '')
    expect(rows.map((row) => row.key)).toEqual(['prj_a', 'prj_b', 'prj_c'])
    expect(rows[0].target).toEqual({ projectId: 'prj_a' })
  })

  it('names a project that has none', () => {
    expect(buildNewChatRows(projects, '')[2].name).toBe('Untitled project')
  })

  it('matches every token, not the raw string — "hook relay" finds webhookrelay', () => {
    expect(buildNewChatRows(projects, 'hook relay').map((row) => row.key)).toEqual(['prj_b'])
  })

  it('searches the detail line too', () => {
    expect(buildNewChatRows(projects, 'helixml').map((row) => row.key)).toEqual(['prj_a'])
  })

  it('returns nothing when nothing matches', () => {
    expect(buildNewChatRows(projects, 'zzz')).toEqual([])
  })
})

describe('NewChatProjectDialog', () => {
  const renderDialog = (onSelect = vi.fn(), onClose = vi.fn(), onCreateProject = vi.fn()) => {
    render(
      <NewChatProjectDialog
        open
        projects={projects}
        onClose={onClose}
        onCreateProject={onCreateProject}
        onSelect={onSelect}
      />,
    )
    return { onSelect, onClose, onCreateProject }
  }

  it('opens project creation from the picker', () => {
    const { onClose, onCreateProject } = renderDialog()

    fireEvent.click(screen.getByRole('button', { name: 'New project' }))

    expect(onClose).toHaveBeenCalled()
    expect(onCreateProject).toHaveBeenCalled()
  })

  it('starts the chat in the project that was clicked', () => {
    const { onSelect, onClose } = renderDialog()

    fireEvent.click(screen.getByLabelText('New task in home stuff'))

    expect(onSelect).toHaveBeenCalledWith({ projectId: 'prj_a' })
    expect(onClose).toHaveBeenCalled()
  })

  it('arrows down the list and picks with Enter', () => {
    const { onSelect } = renderDialog()
    const search = screen.getByLabelText('Search projects')

    fireEvent.keyDown(search, { key: 'ArrowDown' })
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(onSelect).toHaveBeenCalledWith({ projectId: 'prj_b' })
  })

  it('scrolls the highlighted row into view as the cursor moves', () => {
    const scrollIntoView = vi.fn()
    // jsdom has no scrollIntoView; stand one in so we can assert it fires.
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    })
    try {
      renderDialog()
      const search = screen.getByLabelText('Search projects')
      scrollIntoView.mockClear()
      fireEvent.keyDown(search, { key: 'ArrowDown' })
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'nearest' })
    } finally {
      delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView
    }
  })

  it('wraps around the ends rather than sticking', () => {
    const { onSelect } = renderDialog()
    const search = screen.getByLabelText('Search projects')

    // Up from the first row lands on the last.
    fireEvent.keyDown(search, { key: 'ArrowUp' })
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(onSelect).toHaveBeenCalledWith({ projectId: 'prj_c' })
  })

  it('filters as you type', () => {
    renderDialog()

    fireEvent.change(screen.getByLabelText('Search projects'), { target: { value: 'webhook' } })

    expect(screen.getByLabelText('New task in webhookrelay')).toBeInTheDocument()
    expect(screen.queryByLabelText('New task in home stuff')).not.toBeInTheDocument()
  })

  it('keeps Enter safe when the filter empties the list', () => {
    const { onSelect } = renderDialog()
    const search = screen.getByLabelText('Search projects')

    fireEvent.change(search, { target: { value: 'nothing matches this' } })
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(onSelect).not.toHaveBeenCalled()
  })

  it('shows the keyboard legend and closes with Backspace from an empty search', () => {
    const { onClose } = renderDialog()
    const search = screen.getByLabelText('Search projects')

    expect(screen.getByText('Navigate')).toBeInTheDocument()
    expect(screen.getByText('Select')).toBeInTheDocument()
    expect(screen.getByText('Back')).toBeInTheDocument()
    expect(screen.getByText('Close')).toBeInTheDocument()

    fireEvent.keyDown(search, { key: 'Backspace' })

    expect(onClose).toHaveBeenCalled()
  })

  it('keeps Backspace available for editing a non-empty search', () => {
    const { onClose } = renderDialog()
    const search = screen.getByLabelText('Search projects')

    fireEvent.change(search, { target: { value: 'webhook' } })
    fireEvent.keyDown(search, { key: 'Backspace' })

    expect(onClose).not.toHaveBeenCalled()
  })
})
