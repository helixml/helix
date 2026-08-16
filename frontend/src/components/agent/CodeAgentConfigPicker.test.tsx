import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ReactElement, useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
} from '../../api/api'
import CodeAgentConfigPicker from './CodeAgentConfigPicker'

const harnessState = vi.hoisted(() => ({ harnesses: [] as any[] }))
const providerState = vi.hoisted(() => ({ providers: [] as any[] }))
const navigate = vi.hoisted(() => vi.fn())

vi.mock('../../services/orgService', () => ({
  useGetOrgByName: () => ({ data: { id: 'org-1' }, isLoading: false }),
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

describe('CodeAgentConfigPicker', () => {
  beforeEach(() => {
    navigate.mockClear()
    providerState.providers = [
      {
        id: 'provider-1',
        name: 'OpenAI',
        available_models: [{ id: 'api-model', enabled: true, type: 'text' }],
      },
      {
        id: 'provider-2',
        name: 'Anthropic',
        available_models: [{ id: 'claude-api-model', enabled: true, type: 'chat' }],
      },
    ]
    harnessState.harnesses = [
      TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
      TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
    ].map((runtime) => ({
      runtime,
      enabled: true,
      supports_subscription: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        || runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      viewer_has_subscription: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
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
    expect(screen.queryByRole('button', { name: 'Codex' })).not.toBeInTheDocument()
  })

  it('offers models from every allowed provider without org model pinning', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.getByText('api-model')).toBeInTheDocument()
    expect(screen.getByText('claude-api-model')).toBeInTheDocument()
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
  })

  it('hides models from providers disabled for the selected harness', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
        ? { ...harness, provider_refs: ['provider-2'] }
        : harness)
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.queryByText('api-model')).not.toBeInTheDocument()
    expect(screen.queryByText('OpenAI')).not.toBeInTheDocument()
    expect(screen.getByText('claude-api-model')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
  })

  it('combines models from settings-enabled subscription and API sources', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByText('Claude Opus 5 (1M context, recommended)')).toBeInTheDocument()
    expect(screen.getAllByText('Claude subscription').length).toBeGreaterThan(0)
    expect(screen.getByText('api-model')).toBeInTheDocument()
    expect(screen.queryByText('Credentials')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.queryByText('GPT-5.6 Sol')).not.toBeInTheDocument()
    expect(screen.getByText('api-model')).toBeInTheDocument()
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
    expect(screen.getByText('api-model')).toBeInTheDocument()
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
    }))
  })

  it('defaults a new task to Claude Opus when Claude subscription is available', async () => {
    const onChange = vi.fn()
    const AutoSelectingPicker = () => {
      const [value, setValue] = useState<TypesCodeAgentExecutionConfig>()
      return (
        <CodeAgentConfigPicker
          value={value}
          autoSelectSubscriptionDefault
          onChange={(next) => {
            onChange(next)
            setValue(next)
          }}
        />
      )
    }
    renderPicker(<AutoSelectingPicker />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'claude-opus-5',
    }))
    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('Claude Opus 5')
  })

  it('defaults a new task to GPT-5.6 Sol when only Codex subscription is available', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) => ({
      ...harness,
      viewer_has_subscription: harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
    }))
    const onChange = vi.fn()
    renderPicker(
      <CodeAgentConfigPicker
        autoSelectSubscriptionDefault
        onChange={onChange}
      />,
    )

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'gpt-5.6-sol',
    }))
  })

  it('does not auto-select on surfaces that require an explicit choice', async () => {
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).not.toHaveBeenCalled())
  })

  it('writes the provider and model selected in chat into the task config', () => {
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByText('claude-api-model'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-2',
      model: 'claude-api-model',
    }))
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

  it('does not advertise a stored config from a disabled provider', () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        ? { ...harness, provider_refs: ['provider-2'] }
        : harness)
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

    expect(screen.getByRole('button', { name: 'Change coding agent' })).toHaveTextContent('Configure harness')
  })

  it('replaces a disabled legacy project default on a new-task surface', async () => {
    harnessState.harnesses = harnessState.harnesses.map((harness) =>
      harness.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
        ? { ...harness, enabled: false }
        : harness)
    const onChange = vi.fn()
    renderPicker(
      <CodeAgentConfigPicker
        autoSelectSubscriptionDefault
        onChange={onChange}
        value={{
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
    }))
  })
})
