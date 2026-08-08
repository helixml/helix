import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import AgentHarness, { getAgentHarnessLabel, getAgentHarnessModel, getAgentHarnessRuntime } from './AgentHarness'

describe('AgentHarness', () => {
  it('renders the long form with the harness name', () => {
    render(<AgentHarness runtime="claude_code" variant="long" />)

    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('renders the short form as an accessible brand mark', () => {
    render(<AgentHarness runtime="codex_cli" variant="short" />)

    expect(screen.getByRole('img', { name: 'Codex' })).toBeInTheDocument()
    expect(screen.queryByText('Codex')).not.toBeInTheDocument()
  })

  it.each([
    ['zed_agent', 'Zed Agent'],
    ['qwen_code', 'Qwen Code'],
    ['goose_code', 'Goose'],
  ])('renders the official %s harness mark', (runtime, label) => {
    render(<AgentHarness runtime={runtime} variant="short" />)

    const mark = screen.getByRole('img', { name: label })
    expect(mark.querySelector(`[data-harness-mark="${runtime}"]`)).toBeInTheDocument()
  })

  it('uses a readable label for unknown runtimes', () => {
    expect(getAgentHarnessLabel('future_harness')).toBe('Future Harness')
  })

  it('resolves the runtime from an agent and defaults to Zed', () => {
    expect(getAgentHarnessRuntime({
      config: { helix: { assistants: [{ code_agent_runtime: 'claude_code' }] } },
    })).toBe('claude_code')
    expect(getAgentHarnessRuntime()).toBe('zed_agent')
  })

  it('resolves the model selected by a harness', () => {
    expect(getAgentHarnessModel({
      config: { helix: { assistants: [{ code_agent_runtime: 'codex_cli', model: 'gpt-5.6-sol' }] } },
    })).toBe('gpt-5.6-sol')
    expect(getAgentHarnessModel({
      config: { helix: { assistants: [{
        code_agent_runtime: 'claude_code',
        code_agent_credential_type: 'subscription',
        claude_subscription_model: 'opus[1m]',
        model: 'stale-model',
      }] } },
    })).toBe('opus[1m]')
  })
})
