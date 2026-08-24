import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ReactElement, useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
  TypesProviderEndpointType,
  TypesProviderEndpointStatus,
} from '../../api/api'
import CodeAgentConfigPicker from './CodeAgentConfigPicker'
import { CodeAgentConfigChangeSource } from '../../utils/codeAgentExecutionConfig'

const harnessState = vi.hoisted(() => ({ harnesses: [] as any[] }))
const providerState = vi.hoisted(() => ({ providers: [] as any[] }))
const navigate = vi.hoisted(() => vi.fn())

vi.mock('../../services/orgService', () => ({
  useGetOrgByName: () => ({ data: { id: 'org-1', display_name: 'Probably' }, isLoading: false }),
}))
vi.mock('../../services/providersService', () => ({
  useListProviders: () => ({ data: providerState.providers, isLoading: false }),
}))
vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ params: { org_id: 'test-org' }, navigate }),
}))
vi.mock('../account/ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: [] }),
}))
vi.mock('../../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: [] }),
}))
vi.mock('../../services/codeAgentHarnessesService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/codeAgentHarnessesService')>()),
  useOrgCodeAgentHarnesses: () => ({
    data: harnessState.harnesses,
    isLoading: false,
    isFetching: false,
  }),
}))

function renderPicker(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

function AutoSelectingPicker({
  onChange,
  initial,
}: {
  onChange?: (value: TypesCodeAgentExecutionConfig, source: CodeAgentConfigChangeSource) => void
  initial?: TypesCodeAgentExecutionConfig
}) {
  const [value, setValue] = useState<TypesCodeAgentExecutionConfig | undefined>(initial)
  return (
    <CodeAgentConfigPicker
      value={value}
      autoSelectDefault
      onChange={(next, source) => {
        onChange?.(next, source)
        setValue(next)
      }}
    />
  )
}

describe('CodeAgentConfigPicker', () => {
  beforeEach(() => {
    navigate.mockClear()
    providerState.providers = [
      {
        id: 'provider-1',
        name: 'OpenAI',
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeGlobal,
        status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
        available_models: [
          { id: 'api-model', enabled: true, type: 'text' },
          { id: 'gpt-5.6-sol', enabled: true, type: 'text' },
          { id: 'gpt-5.6-terra', enabled: true, type: 'text' },
        ],
      },
      {
        id: 'provider-2',
        name: 'Anthropic',
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeGlobal,
        status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
        available_models: [
          { id: 'claude-api-model', enabled: true, type: 'chat' },
          { id: 'claude-opus-5', enabled: true, type: 'chat' },
          { id: 'claude-fable-5', enabled: true, type: 'chat' },
        ],
      },
    ]
    harnessState.harnesses = [
      TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
      TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeQwenCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
    ].map((runtime) => ({
      runtime,
      enabled: true,
      supports_subscription: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        || runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      viewer_has_subscription: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      subscription_enabled: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
    }))
  })

  it('offers only harnesses enabled by the organization', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI
        ? { ...harness, enabled: false }
        : harness)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    expect(screen.getByRole('button', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Qwen Code' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Codex' })).not.toBeInTheDocument()
  })

  it('offers models from every allowed provider without org model pinning', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.getByText('api-model')).toBeInTheDocument()
    expect(screen.getByText('claude-api-model')).toBeInTheDocument()
    expect(screen.getAllByText('Global / OpenAI').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Global / Anthropic').length).toBeGreaterThan(0)
  })

  it('hides models from providers disabled for the selected harness', () => {
    providerState.providers = providerState.providers.map((provider) => ({
      ...provider,
      endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
    }))
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
        ? { ...harness, provider_refs: ['provider-2'] }
        : harness)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.queryByText('api-model')).not.toBeInTheDocument()
    expect(screen.queryByText('Probably / OpenAI')).not.toBeInTheDocument()
    expect(screen.getByText('claude-api-model')).toBeInTheDocument()
    expect(screen.getAllByText('Probably / Anthropic').length).toBeGreaterThan(0)
  })

  it('treats an explicit empty provider list as API providers disabled', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
        ? { ...harness, provider_refs: [] }
        : harness)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.queryByText('api-model')).not.toBeInTheDocument()
    expect(screen.queryByText('claude-api-model')).not.toBeInTheDocument()
  })

  it('does not expose cached models from a disconnected provider', () => {
    providerState.providers = providerState.providers.map((provider) =>
      provider.name === 'Anthropic'
        ? { ...provider, status: TypesProviderEndpointStatus.ProviderEndpointStatusError }
        : provider)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))

    expect(screen.queryByText('claude-opus-5')).not.toBeInTheDocument()
    expect(screen.queryByText('claude-fable-5')).not.toBeInTheDocument()
  })

  it('exposes only the credential mode selected in organization settings', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByText('Claude Opus 5 (1M context, recommended)')).toBeInTheDocument()
    expect(screen.getAllByText('Claude subscription').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Claude Fable 5/).length).toBeGreaterThan(0)
    expect(screen.queryByText('api-model')).not.toBeInTheDocument()
    expect(screen.queryByText('claude-api-model')).not.toBeInTheDocument()
    expect(screen.queryByText('Credentials')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.queryByText('GPT-5.6 Sol')).not.toBeInTheDocument()
    expect(screen.getByText('gpt-5.6-terra')).toBeInTheDocument()
    expect(screen.queryByText('claude-fable-5')).not.toBeInTheDocument()
  })

  it('hides subscription models when that source is disabled in settings', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        ? { ...harness, subscription_enabled: false }
        : harness)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))

    expect(screen.queryByText('Claude Opus 5 (1M context, recommended)')).not.toBeInTheDocument()
    expect(screen.getByText('claude-fable-5')).toBeInTheDocument()
  })

  it('stores subscription credentials when a subscription model is selected', () => {
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    fireEvent.click(screen.getByText('Claude Opus 5 (1M context, recommended)'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      provider_ref: undefined,
      model: 'claude-opus-5',
    }), 'user')
  })

  it('defaults a new task to Claude Opus when Claude subscription is available', async () => {
    const onChange = vi.fn()
    renderPicker(<AutoSelectingPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'claude-opus-5',
    }, 'auto'))
    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('Claude Opus 5')
  })

  it('defaults a new task to GPT-5.6 Sol when only Codex subscription is available', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      subscription_enabled: harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
    }))
    const onChange = vi.fn()
    renderPicker(<AutoSelectingPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'gpt-5.6-sol',
    }, 'auto'))
  })

  it('defaults a new task to Claude Opus from a connected Anthropic provider', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: false,
      subscription_enabled: false,
    }))
    const onChange = vi.fn()
    renderPicker(<AutoSelectingPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-2',
      model: 'claude-opus-5',
    }, 'auto'))
    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('claude-opus-5')
  })

  it('keeps the last known harness when it is still available', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: false,
      subscription_enabled: false,
    }))
    const onChange = vi.fn()
    renderPicker(
      <AutoSelectingPicker
        onChange={onChange}
        initial={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('api-model')
    })
    expect(onChange).not.toHaveBeenCalled()
  })

  it('replaces a stored API config when that provider is disconnected', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: false,
      subscription_enabled: false,
    }))
    providerState.providers = providerState.providers.map((provider) =>
      provider.name === 'Anthropic'
        ? { ...provider, status: TypesProviderEndpointStatus.ProviderEndpointStatusError }
        : provider)
    const onChange = vi.fn()
    renderPicker(
      <AutoSelectingPicker
        onChange={onChange}
        initial={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-2',
          model: 'claude-opus-5',
        }}
      />,
    )

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-1',
      model: 'gpt-5.6-sol',
    }, 'auto'))
  })

  it('writes the provider and model selected in chat into the task config', () => {
    providerState.providers = providerState.providers.map((provider) =>
      provider.id === 'provider-2'
        ? { ...provider, name: 'user/anthropic', endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg }
        : provider)
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        ? { ...harness, subscription_enabled: false }
        : harness)
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    const option = screen.getByRole('button', { name: 'Select claude-fable-5 from Probably / Anthropic' })
    expect(option).toHaveTextContent('Probably / Anthropic')
    fireEvent.click(option)

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-2',
      model: 'claude-fable-5',
    }), 'user')
  })

  it('shows duplicate vendor scopes and stores the selected endpoint ID', () => {
    providerState.providers = [
      {
        id: 'pe_org_anthropic',
        name: 'user/anthropic',
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
        status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
        available_models: [{ id: 'org-claude', enabled: true, type: 'chat' }],
      },
      {
        id: 'global/anthropic',
        name: 'anthropic',
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeGlobal,
        status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
        available_models: [{ id: 'global-claude', enabled: true, type: 'chat' }],
      },
    ]
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Zed Agent' }))

    expect(screen.getByText('Probably / Anthropic')).toBeInTheDocument()
    expect(screen.getByText('Global / Anthropic')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Select global-claude from Global / Anthropic' }))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      provider_ref: 'global/anthropic',
      model: 'global-claude',
    }), 'user')
  })

  it('keeps older native models in a collapsed legacy section', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))

    expect(screen.getByText('Claude Opus 5 (1M context, recommended)')).toBeInTheDocument()
    expect(screen.getAllByText(/Claude Fable 5/).length).toBeGreaterThan(0)
    expect(screen.queryByText('Claude Opus 4.8 (1M context)')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show Legacy models' }))
    expect(screen.getByText('Claude Opus 4.8 (1M context)')).toBeInTheDocument()
    expect(screen.queryByText('claude-api-model')).not.toBeInTheDocument()
  })

  it('routes to organization settings when no harness is enabled', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({ ...harness, enabled: false }))
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    const dialog = within(await screen.findByRole('dialog'))
    fireEvent.click(dialog.getByRole('button', { name: 'Configure' }))
    expect(navigate).toHaveBeenCalledWith('org_providers', { org_id: 'test-org' })
  })

  it('does not advertise a stored config for a disabled harness', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({ ...harness, enabled: false }))
    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    const trigger = screen.getByRole('button', { name: 'Change coding agent' })
    expect(trigger).toHaveTextContent('Configure harness')
    expect(trigger).not.toHaveTextContent('api-model')
  })

  it('does not mutate an existing task whose provider is no longer allowed', async () => {
    providerState.providers = providerState.providers.map((provider) => ({
      ...provider,
      endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
    }))
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        ? { ...harness, subscription_enabled: false, provider_refs: ['provider-2'] }
        : harness)
    const onChange = vi.fn()
    renderPicker(
      <CodeAgentConfigPicker
        onChange={onChange}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    await waitFor(() => expect(onChange).not.toHaveBeenCalled())
    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('Configure harness')
  })

  it('picks another harness when the last known one is no longer enabled', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
        ? { ...harness, enabled: false }
        : harness)
    const onChange = vi.fn()
    renderPicker(
      <AutoSelectingPicker
        onChange={onChange}
        initial={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'claude-opus-5',
    }, 'auto'))
  })

  it('defaults to Codex Sol when OpenAI is the only connected native provider', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      enabled: harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      viewer_has_subscription: false,
    }))
    providerState.providers = providerState.providers.map((provider) =>
      provider.name === 'Anthropic'
        ? { ...provider, status: TypesProviderEndpointStatus.ProviderEndpointStatusError }
        : provider)
    const onChange = vi.fn()
    renderPicker(<AutoSelectingPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-1',
      model: 'gpt-5.6-sol',
    }, 'auto'))
    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('gpt-5.6-sol')
  })
})
