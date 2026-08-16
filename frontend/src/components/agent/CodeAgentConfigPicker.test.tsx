import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
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

  it('offers models from every configured provider without org model pinning', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    expect(screen.getByText('api-model')).toBeInTheDocument()
    expect(screen.getByText('claude-api-model')).toBeInTheDocument()
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
  })

  it('keeps subscription versus API provider selection in the task picker for Claude and Codex', () => {
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByRole('radio', { name: 'Claude subscription' })).toBeEnabled()
    expect(screen.getByRole('radio', { name: 'API provider' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.getByRole('radio', { name: 'ChatGPT subscription' })).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'API provider' })).toBeChecked()
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
})
