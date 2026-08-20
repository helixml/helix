import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ArtifactDialog from './ArtifactDialog'

describe('ArtifactDialog', () => {
  it('offers a project-specific agent prompt alongside manual upload', () => {
    render(
      <ArtifactDialog
        open
        projectName="Example project"
        saving={false}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )

    expect(screen.getByText('Upload manually')).toBeInTheDocument()
    expect(screen.getByText('Or')).toBeInTheDocument()
    expect(screen.getByText('Create with an agent')).toBeInTheDocument()
    expect(screen.getByText('Create and upload an artifact to the Helix project "Example project".')).toBeInTheDocument()
    expect(document.querySelector('[data-chat-code-block]')).toBeInTheDocument()
    expect(document.querySelector('[data-chat-code-block]')).toHaveAttribute('data-wrap', 'true')
    expect(screen.getByRole('button', { name: 'Disable line wrap' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy code' })).toBeInTheDocument()
  })

  it('accepts PDF and image uploads without asking for an HTML entrypoint', () => {
    render(
      <ArtifactDialog
        open
        projectName="Example project"
        saving={false}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )

    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    expect(input.accept).toContain('.pdf')
    expect(input.accept).toContain('image/*')
    expect(screen.getByLabelText('Entrypoint')).toBeInTheDocument()

    fireEvent.change(input, { target: { files: [new File(['pdf'], 'report.pdf', { type: 'application/pdf' })] } })

    expect(screen.getByText('report.pdf')).toBeInTheDocument()
    expect(screen.queryByLabelText('Entrypoint')).not.toBeInTheDocument()
  })
})
