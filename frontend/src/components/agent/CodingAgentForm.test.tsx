import { createRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import CodingAgentForm, { CodingAgentFormHandle } from './CodingAgentForm'

vi.mock('../../hooks/useApps', () => ({
  default: () => ({ createAgent: vi.fn() }),
}))

vi.mock('./useCodingAgentProviderState', () => ({
  useCodingAgentProviderState: () => ({
    hasAnthropicProvider: false,
    hasClaudeSubscription: true,
    hasCodexSubscription: false,
  }),
}))

describe('CodingAgentForm', () => {
  it('returns the selected Claude subscription model', () => {
    const ref = createRef<CodingAgentFormHandle>()
    render(
      <CodingAgentForm
        ref={ref}
        value={{
          codeAgentRuntime: 'claude_code',
          claudeCodeMode: 'subscription',
          selectedProvider: '',
          selectedModel: '',
          agentName: 'Claude Code',
        }}
        onChange={vi.fn()}
        recommendedModels={[]}
        showAgentName={false}
        showModelSelection={false}
        showCreateButton={false}
      />,
    )

    fireEvent.mouseDown(screen.getAllByRole('combobox')[1])
    fireEvent.click(screen.getByRole('option', { name: /Claude Fable 5/i }))

    expect(ref.current?.handleGetConfig()).toEqual({
      runtime: 'claude_code',
      credential_type: 'subscription',
      provider_ref: undefined,
      model: 'claude-fable-5',
    })
  })
})
