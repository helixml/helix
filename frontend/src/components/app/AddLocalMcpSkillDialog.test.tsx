import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { TypesAssistantMCP } from '../../api/api'
import { IAppFlatState } from '../../types'
import AddLocalMcpSkillDialog from './AddLocalMcpSkillDialog'

describe('AddLocalMcpSkillDialog', () => {
  it('masks existing environment variable values until explicitly revealed', () => {
    const skill: TypesAssistantMCP = {
      name: 'Private MCP',
      transport: 'stdio',
      command: 'private-mcp',
      env: { API_TOKEN: 'screen-share-secret' },
    }

    render(
      <AddLocalMcpSkillDialog
        open
        onClose={vi.fn()}
        skill={skill}
        app={{ mcpTools: [skill] } as IAppFlatState}
        onUpdate={vi.fn()}
        isEnabled
      />,
    )

    const valueInput = screen.getByPlaceholderText('Value')
    expect(valueInput).toHaveAttribute('type', 'password')

    fireEvent.click(screen.getByRole('button', { name: 'Show value for API_TOKEN' }))
    expect(valueInput).toHaveAttribute('type', 'text')
    expect(screen.getByRole('button', { name: 'Hide value for API_TOKEN' })).toBeInTheDocument()
  })
})
