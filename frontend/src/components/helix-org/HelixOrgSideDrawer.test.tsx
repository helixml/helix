import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import HelixOrgSideDrawer from './HelixOrgSideDrawer'

describe('HelixOrgSideDrawer', () => {
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
