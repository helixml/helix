import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Checkbox from '@mui/material/Checkbox'
import FormControlLabel from '@mui/material/FormControlLabel'

import SimpleConfirmWindow from './SimpleConfirmWindow'

// The archive dialog defaults its checkbox to "on": archiving stops the agent
// and frees the sandbox, which is what a user clearing a finished lane expects.
// These cover the two properties that matter: the option starts checked, and
// what the dialog submits reflects what the user see.
function ArchiveDialog({ onSubmit }: { onSubmit: (stopAgent: boolean) => void }) {
  const [stopAgent, setStopAgent] = useState(true)
  return (
    <SimpleConfirmWindow
      title="Archive spec task"
      message="Archive “task”? You can restore it from the Archived view."
      confirmTitle="Archive"
      onCancel={() => {}}
      onSubmit={() => onSubmit(stopAgent)}
    >
      <FormControlLabel
        control={(
          <Checkbox
            size="small"
            checked={stopAgent}
            onChange={(e) => setStopAgent(e.target.checked)}
          />
        )}
        label="Stop the agent and free its sandbox"
      />
    </SimpleConfirmWindow>
  )
}

describe('archive dialog stop-agent option', () => {
  it('defaults to stopping the agent and frees the sandbox', () => {
    const onSubmit = vi.fn()
    render(<ArchiveDialog onSubmit={onSubmit} />)

    const checkbox = screen.getByRole('checkbox', { name: /Stop the agent and free its sandbox/ })
    expect(checkbox).toBeChecked()

    fireEvent.click(screen.getByRole('button', { name: 'Archive' }))
    expect(onSubmit).toHaveBeenCalledWith(true)
  })

  it('submits stopAgent=false when the user unchecks it', () => {
    const onSubmit = vi.fn()
    render(<ArchiveDialog onSubmit={onSubmit} />)

    fireEvent.click(screen.getByRole('checkbox', { name: /Stop the agent and free its sandbox/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Archive' }))

    expect(onSubmit).toHaveBeenCalledWith(false)
  })
})
