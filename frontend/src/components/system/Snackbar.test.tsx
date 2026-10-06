import { fireEvent, render, screen, within } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it, vi } from 'vitest'

import { ISnackbarData, SnackbarContext } from '../../contexts/snackbar'
import Snackbar from './Snackbar'

const notifications: ISnackbarData[] = [
  { id: 'older', message: 'Upload started', severity: 'info' },
  { id: 'newest', message: 'Settings saved', severity: 'success' },
]

function renderSnackbar(items = notifications) {
  const dismissSnackbar = vi.fn()

  render(
    <ThemeProvider theme={createTheme({ palette: { mode: 'dark' } })}>
      <SnackbarContext.Provider
        value={{
          snackbars: items,
          setSnackbar: vi.fn(),
          dismissSnackbar,
        }}
      >
        <Snackbar />
      </SnackbarContext.Provider>
    </ThemeProvider>,
  )

  return dismissSnackbar
}

describe('Snackbar', () => {
  it('layers notifications in the bottom-left with the newest on top', () => {
    renderSnackbar()

    const region = screen.getByLabelText('Notifications')
    const cards = within(region).getAllByRole('status')
    expect(region).toHaveAttribute('data-expanded', 'false')
    expect(cards[0]).toHaveAttribute('data-notification-id', 'older')
    expect(cards[1]).toHaveAttribute('data-notification-id', 'newest')
    expect(cards[1]).toHaveAttribute('data-stack-index', '1')
  })

  it('expands the stack for inspection on hover', () => {
    renderSnackbar()

    const region = screen.getByLabelText('Notifications')
    fireEvent.mouseEnter(region)

    expect(region).toHaveAttribute('data-expanded', 'true')
    expect(screen.getByText('Upload started')).toBeVisible()
    expect(screen.getByText('Settings saved')).toBeVisible()
  })

  it('dismisses an individual notification', () => {
    const dismissSnackbar = renderSnackbar()

    fireEvent.click(screen.getByRole('button', {
      name: 'Dismiss notification: Upload started',
    }))

    expect(dismissSnackbar).toHaveBeenCalledWith('older')
  })

  it('uses an alert role for urgent notifications', () => {
    renderSnackbar([
      { id: 'error', message: 'Save failed', severity: 'error' },
    ])

    expect(screen.getByRole('alert')).toHaveTextContent('Save failed')
  })
})
