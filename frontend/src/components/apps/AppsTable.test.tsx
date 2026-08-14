import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ConfigurationWarningAlert, ConfigurationWarningChip } from './ConfigurationWarning'

describe('agent configuration warnings', () => {
  it('shows the compact list warning and detail alert', () => {
    render(
      <>
        <ConfigurationWarningChip warning="Select a valid provider" />
        <ConfigurationWarningAlert warning="Select a valid provider" />
      </>,
    )

    expect(screen.getByText('Needs attention')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('Select a valid provider')
  })

  it('renders nothing when the server sends no warning', () => {
    const { container } = render(
      <>
        <ConfigurationWarningChip />
        <ConfigurationWarningAlert />
      </>,
    )

    expect(container).toBeEmptyDOMElement()
  })
})
