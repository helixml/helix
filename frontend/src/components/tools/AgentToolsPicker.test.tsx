import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import AgentToolsPicker from './AgentToolsPicker'

vi.mock('../../services/agentToolsService', () => ({
  useAgentToolCatalogue: () => ({
    data: [
      { name: 'create_spectask', description: 'Create a spec task.' },
      { name: 'list_spectasks', description: 'List spec tasks.' },
    ],
  }),
}))

describe('AgentToolsPicker', () => {
  it('summarises the grant as a count rather than listing every tool', () => {
    render(
      <AgentToolsPicker
        label="Agent tools"
        selectedTools={['list_spectasks']}
        lockedTools={['create_spectask']}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByText('2 tools enabled')).toBeTruthy()
    // The tool names belong in the dialog, not in the panel.
    expect(screen.queryByText('create_spectask')).toBeNull()
  })

  it('singularises a one-tool grant', () => {
    render(<AgentToolsPicker selectedTools={['list_spectasks']} onChange={vi.fn()} />)
    expect(screen.getByText('1 tool enabled')).toBeTruthy()
  })
})
