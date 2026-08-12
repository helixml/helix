import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import ToolPickerDialog, { groupToolCatalogue, toolCapabilityGroupKey } from './ToolPickerDialog'

const catalogue = [
  { name: 'create_bot', description: 'Create an agent.' },
  { name: 'publish', description: 'Publish an event.' },
  { name: 'list_sandboxes', description: 'List standalone sandboxes.' },
  { name: 'create_sandbox', description: 'Create a standalone sandbox.' },
  { name: 'server_run_command', description: 'Run a command on a server.' },
  { name: 'future_capability', description: 'A newly registered capability.' },
]

const openSection = (name: string) => {
  const summary = screen.getByText(name).closest('[role="button"]')
  expect(summary).toBeTruthy()
  fireEvent.click(summary!)
}

describe('ToolPickerDialog', () => {
  it('groups known tools and leaves new tools discoverable in Other tools', () => {
    expect(toolCapabilityGroupKey('create_bot')).toBe('agents')
    expect(toolCapabilityGroupKey('create_sandbox')).toBe('sandboxes')
    expect(toolCapabilityGroupKey('server_run_command')).toBe('infrastructure')
    expect(toolCapabilityGroupKey('future_capability')).toBe('other')

    const groups = groupToolCatalogue(catalogue)
    expect(groups.find((group) => group.key === 'sandboxes')?.tools.map((tool) => tool.name)).toEqual([
      'create_sandbox',
      'list_sandboxes',
    ])
    expect(groups.find((group) => group.key === 'other')?.tools[0].name).toBe('future_capability')
  })

  it('edits a draft selection and applies it without saving on each click', () => {
    const onApply = vi.fn()
    const onClose = vi.fn()
    render(
      <ToolPickerDialog
        open
        tools={catalogue}
        selectedTools={['list_sandboxes']}
        onApply={onApply}
        onClose={onClose}
      />,
    )

    expect(screen.getByText('Create and administer standalone organization containers independently from agent desktops.')).toBeTruthy()
    expect(screen.queryByRole('checkbox', { name: 'Enable list_sandboxes' })).not.toBeInTheDocument()
    openSection('Sandbox environments')
    expect(screen.getByRole('checkbox', { name: 'Enable list_sandboxes' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Enable create_sandbox' })).not.toBeChecked()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Enable create_sandbox' }))
    expect(onApply).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Apply selection' }))

    expect(onApply).toHaveBeenCalledWith(['create_sandbox', 'list_sandboxes'])
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('can enable or disable a complete section', () => {
    const onApply = vi.fn()
    render(
      <ToolPickerDialog
        open
        tools={catalogue}
        selectedTools={[]}
        onApply={onApply}
        onClose={vi.fn()}
      />,
    )

    openSection('Sandbox environments')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Toggle all Sandbox environments tools' }))
    expect(screen.getByRole('checkbox', { name: 'Enable create_sandbox' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Enable list_sandboxes' })).toBeChecked()
    fireEvent.click(screen.getByRole('button', { name: 'Apply selection' }))
    expect(onApply).toHaveBeenCalledWith(['create_sandbox', 'list_sandboxes'])
  })
})
