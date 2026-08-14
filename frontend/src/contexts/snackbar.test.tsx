import { fireEvent, render, screen, within } from '@testing-library/react'
import React from 'react'
import { describe, expect, it } from 'vitest'

import { SnackbarContext, SnackbarContextProvider } from './snackbar'

const SnackbarHarness = () => {
  const { snackbars, setSnackbar, dismissSnackbar } = React.useContext(SnackbarContext)

  return (
    <>
      <button onClick={() => setSnackbar('First', 'info')}>Add first</button>
      <button onClick={() => setSnackbar('Second', 'success')}>Add second</button>
      <ol aria-label="Queued notifications">
        {snackbars.map((snackbar) => (
          <li key={snackbar.id}>
            {snackbar.message}
            <button onClick={() => dismissSnackbar(snackbar.id)}>Dismiss</button>
          </li>
        ))}
      </ol>
    </>
  )
}

describe('SnackbarContextProvider', () => {
  it('queues notifications and dismisses them independently', () => {
    render(
      <SnackbarContextProvider>
        <SnackbarHarness />
      </SnackbarContextProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Add first' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add second' }))

    const queue = screen.getByRole('list', { name: 'Queued notifications' })
    const items = within(queue).getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent('First')
    expect(items[1]).toHaveTextContent('Second')

    fireEvent.click(within(items[0]).getByRole('button'))

    expect(within(queue).queryByText(/First/)).not.toBeInTheDocument()
    expect(within(queue).getByText(/Second/)).toBeInTheDocument()
  })
})
