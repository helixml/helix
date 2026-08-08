import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import AgentRuntimeProviderIcon from './AgentRuntimeProviderIcon'

describe('AgentRuntimeProviderIcon', () => {
  it('maps Claude Code and Codex to their harness marks', () => {
    const { rerender } = render(<AgentRuntimeProviderIcon runtime="claude_code" />)
    expect(screen.getByRole('img', { name: 'Claude Code' })).toBeInTheDocument()

    rerender(<AgentRuntimeProviderIcon runtime="codex_cli" />)
    expect(screen.getByRole('img', { name: 'Codex' })).toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'Claude Code' })).not.toBeInTheDocument()
  })

  it('renders a colored runtime icon for other harnesses', () => {
    render(<AgentRuntimeProviderIcon runtime="zed_agent" />)
    expect(screen.getByRole('img', { name: 'Zed Agent' })).toBeInTheDocument()
  })
})
