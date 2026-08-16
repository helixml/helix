import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
} from '../../api/api'
import CodeAgentHarnessesSection from './CodeAgentHarnessesSection'

const harnesses: TypesOrgCodeAgentHarnessStatus[] = [{
  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  enabled: true,
  supports_subscription: true,
  viewer_has_subscription: true,
}]

describe('CodeAgentHarnessesSection', () => {
  it('expands a harness to show provider controls without exposing model policy', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          available_models: [{ id: 'task-selected-model', enabled: true, type: 'chat' }],
        }]}
        subscriptionIdentity={() => 'developer@example.com · Claude Max Subscription'}
        subscriptionAction={() => <button>Manage subscription</button>}
        onChange={vi.fn()}
      />,
    )

    expect(screen.queryByText('anthropic')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))

    expect(screen.getByText('anthropic')).toBeInTheDocument()
    expect(screen.getByText('developer@example.com · Claude Max Subscription')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
    expect(screen.queryByText('task-selected-model')).not.toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Disable subscription for Claude Code' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Disable anthropic for Claude Code' })).toBeChecked()
    expect(screen.getByText('API providers')).toBeInTheDocument()
    expect(screen.getByRole('button', {
      name: 'Enabled API providers expose their models in the task chat, where a model is selected.',
    })).toBeInTheDocument()
  })

  it('writes an explicit provider allow-list when a provider is disabled', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[
          {
            id: 'provider-1',
            name: 'openai',
            available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
          },
          {
            id: 'provider-2',
            name: 'anthropic',
            available_models: [{ id: 'model-2', enabled: true, type: 'chat' }],
          },
          {
            id: 'provider-temporarily-unavailable',
            name: 'anthropic',
            available_models: [],
          },
        ]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    expect(screen.queryByText('openai')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Disable anthropic for Claude Code' }))

    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      provider_refs: ['provider-temporarily-unavailable'],
    })
  })

  it('renders an explicit empty allow-list as all providers disabled', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={[{ ...harnesses[0], provider_refs: [] }]}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    expect(screen.getByRole('checkbox', { name: 'Enable anthropic for Claude Code' })).not.toBeChecked()
  })

  it('updates subscription eligibility in organization settings', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Disable subscription for Claude Code' }))

    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      subscription_enabled: false,
    })
  })

  it('updates only the harness enabled state', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('checkbox', { name: 'Enable Claude Code' }))
    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: false,
    })
  })

  it('keeps the subscription switch in its saved state but disabled when disconnected', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={[{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
          enabled: true,
          supports_subscription: true,
          viewer_has_subscription: false,
        }]}
        endpoints={[{
          id: 'provider-1',
          name: 'openai',
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        subscriptionAction={() => <button type="button">Connect</button>}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Codex settings' }))
    expect(screen.getByText('ChatGPT subscription')).toBeInTheDocument()
    expect(screen.getByText('Not connected')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument()
    const subscriptionSwitch = screen.getByRole('checkbox', { name: 'Disable subscription for Codex' })
    expect(subscriptionSwitch).toBeChecked()
    expect(subscriptionSwitch).toBeDisabled()
    expect(screen.getByText('openai')).toBeInTheDocument()
  })

  it('keeps a disabled-off subscription switch when disconnected', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={[{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
          enabled: true,
          supports_subscription: true,
          viewer_has_subscription: false,
          subscription_enabled: false,
        }]}
        endpoints={[]}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    const subscriptionSwitch = screen.getByRole('checkbox', { name: 'Enable subscription for Claude Code' })
    expect(subscriptionSwitch).not.toBeChecked()
    expect(subscriptionSwitch).toBeDisabled()
  })
})
