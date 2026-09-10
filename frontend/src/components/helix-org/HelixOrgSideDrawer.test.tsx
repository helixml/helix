import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import HelixOrgSideDrawer from './HelixOrgSideDrawer'

describe('HelixOrgSideDrawer', () => {
  it('renders footer actions outside the scrolling content', () => {
    render(
      <HelixOrgSideDrawer
        open
        onClose={vi.fn()}
        title="New bot"
        footer={<button>Create</button>}
      >
        Drawer content
      </HelixOrgSideDrawer>,
    )

    const content = screen.getByText('Drawer content')
    const footer = screen.getByRole('button', { name: 'Create' }).parentElement
    expect(content).not.toBe(footer)
    expect(content?.nextElementSibling).toBe(footer)
  })

  it('closes a persistent drawer on Escape when enabled', () => {
    const onClose = vi.fn()
    render(
      <HelixOrgSideDrawer
        open
        onClose={onClose}
        title="Bot details"
        allowInteractionBehind
        closeOnEscape
      >
        Drawer content
      </HelixOrgSideDrawer>,
    )

    expect(screen.getByText('Drawer content')).toBeInTheDocument()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })
})
