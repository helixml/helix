import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import AgentRuntimeProviderIcon from './AgentRuntimeProviderIcon'

describe('AgentRuntimeProviderIcon', () => {
  it('maps Claude Code to Anthropic and Codex CLI to OpenAI', () => {
    const { rerender } = render(<AgentRuntimeProviderIcon runtime="claude_code" />)
    expect(screen.getByRole('img', { name: 'Anthropic' })).toBeInTheDocument()

    rerender(<AgentRuntimeProviderIcon runtime="codex_cli" />)
    expect(screen.getByRole('img', { name: 'OpenAI' })).toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'Anthropic' })).not.toBeInTheDocument()
  })

  it('does not assign a provider icon to other runtimes', () => {
    render(<AgentRuntimeProviderIcon runtime="zed_agent" />)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})
