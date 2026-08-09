import { fireEvent, render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it, vi } from 'vitest'

import ChatSidebarBrandHeader from './ChatSidebarBrandHeader'

describe('ChatSidebarBrandHeader', () => {
  it('renders the Helix wordmark and collapses the chat panel', () => {
    const onCollapse = vi.fn()

    render(
      <ThemeProvider theme={createTheme()}>
        <ChatSidebarBrandHeader onCollapse={onCollapse} />
      </ThemeProvider>,
    )

    expect(screen.getByText('helix')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Collapse chat panel' }))
    expect(onCollapse).toHaveBeenCalledOnce()
  })
})
