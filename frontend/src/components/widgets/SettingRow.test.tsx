import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import SettingRow from './SettingRow'

describe('SettingRow', () => {
  it('renders the name and explanation beside the control', () => {
    render(
      <SettingRow title="Compute" description="Sandbox size for each task.">
        <button type="button">4 vCPU</button>
      </SettingRow>,
    )

    expect(screen.getByText('Compute')).toBeInTheDocument()
    expect(screen.getByText('Sandbox size for each task.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '4 vCPU' })).toBeInTheDocument()
  })

  it('omits the explanation line when there is nothing to explain', () => {
    const { container } = render(
      <SettingRow title="Agent">
        <button type="button">pick</button>
      </SettingRow>,
    )

    expect(screen.getByText('Agent')).toBeInTheDocument()
    expect(container.querySelectorAll('.MuiTypography-caption')).toHaveLength(0)
  })

  it('marks a required setting', () => {
    render(
      <SettingRow title="Project Name" required>
        <input aria-label="Project name" />
      </SettingRow>,
    )

    // The asterisk is the only cue left once the field's floating label is gone.
    expect(screen.getByText('*')).toBeInTheDocument()
  })
})
