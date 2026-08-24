import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
  TypesProviderEndpointType,
  TypesProviderEndpointStatus,
} from '../../api/api'
import CodeAgentHarnessesSection from './CodeAgentHarnessesSection'

const harnesses: TypesOrgCodeAgentHarnessStatus[] = [{
  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  enabled: true,
  supports_subscription: true,
  viewer_has_subscription: true,
  subscription_enabled: true,
}]

describe('CodeAgentHarnessesSection', () => {
  it('shows inherited providers as enabled controls', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={[{ ...harnesses[0], subscription_enabled: false }]}
        endpoints={[{
          id: 'provider-1',
          name: 'user/anthropic',
          endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        organizationName="Probably"
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    expect(screen.getByText('Inheriting all compatible providers')).toBeInTheDocument()
    expect(screen.getByText('Probably / Anthropic')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Disable Probably / Anthropic for Claude Code' }))
    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      provider_refs: [],
    })
  })

  it('expands a harness to show provider controls without exposing model policy', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
          available_models: [{ id: 'task-selected-model', enabled: true, type: 'chat' }],
        }]}
        subscriptionIdentity={() => 'developer@example.com · Claude Max Subscription'}
        subscriptionAction={() => <button>Manage subscription</button>}
        onChange={vi.fn()}
      />,
    )

    expect(screen.queryByText('Anthropic')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))

    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('developer@example.com · Claude Max Subscription')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
    expect(screen.queryByText('task-selected-model')).not.toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Disable subscription for Claude Code' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Enable Anthropic for Claude Code' })).not.toBeChecked()
    expect(screen.getByText('API providers')).toBeInTheDocument()
    expect(screen.getByRole('button', {
      name: 'API-provider mode is exclusive with subscription mode. Enabled providers expose their models in the task chat.',
    })).toBeInTheDocument()
  })

  it('writes an explicit provider allow-list when a provider is disabled', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={[{
          ...harnesses[0],
          subscription_enabled: false,
          provider_refs: ['provider-2', 'provider-temporarily-unavailable'],
        }]}
        endpoints={[
          {
            id: 'provider-1',
            name: 'openai',
            status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
            available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
          },
          {
            id: 'provider-2',
            name: 'anthropic',
            status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
            available_models: [{ id: 'model-2', enabled: true, type: 'chat' }],
          },
          {
            id: 'provider-temporarily-unavailable',
            name: 'anthropic',
            status: TypesProviderEndpointStatus.ProviderEndpointStatusError,
            available_models: [],
          },
        ]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    expect(screen.queryByText('OpenAI')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Disable Anthropic for Claude Code' }))

    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      provider_refs: ['provider-temporarily-unavailable'],
    })
  })

  it('renders an explicit empty allow-list as all providers disabled', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={[{ ...harnesses[0], subscription_enabled: false, provider_refs: [] }]}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    expect(screen.getByText('No API providers enabled')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Enable Anthropic for Claude Code' })).not.toBeChecked()
  })

  it('disables a provider switch until its API connection is healthy', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusError,
          available_models: [{ id: 'stale-cached-model', enabled: true, type: 'chat' }],
        }]}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    const providerSwitch = screen.getByRole('checkbox', { name: 'Enable Anthropic for Claude Code' })
    expect(providerSwitch).not.toBeChecked()
    expect(providerSwitch).toBeDisabled()
    expect(screen.getByText('Not connected')).toBeInTheDocument()
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

  it('switches atomically from subscription to API-provider mode', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Enable Anthropic for Claude Code' }))

    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      provider_refs: ['provider-1'],
      subscription_enabled: false,
    })
  })

  it('switches atomically from API-provider to subscription mode', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={[{
          ...harnesses[0],
          subscription_enabled: false,
          provider_refs: ['provider-1'],
        }]}
        endpoints={[{
          id: 'provider-1',
          name: 'anthropic',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
          available_models: [{ id: 'model-1', enabled: true, type: 'chat' }],
        }]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Enable subscription for Claude Code' }))

    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: true,
      subscription_enabled: true,
      provider_refs: [],
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

  it('defaults legacy source policy to API mode when disconnected', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={[{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
          enabled: true,
          supports_subscription: true,
          viewer_has_subscription: false,
          provider_refs: ['provider-1'],
        }]}
        endpoints={[{
          id: 'provider-1',
          name: 'openai',
          status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
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
    const subscriptionSwitch = screen.getByRole('checkbox', { name: 'Enable subscription for Codex' })
    expect(subscriptionSwitch).not.toBeChecked()
    expect(subscriptionSwitch).toBeDisabled()
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Disable OpenAI for Codex' })).toBeChecked()
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
