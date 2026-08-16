import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from '../../api/api'
import CodeAgentConfigPicker from './CodeAgentConfigPicker'

const subscriptionState = vi.hoisted(() => ({ claude: [] as any[], codex: [] as any[] }))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ params: { org_id: 'test-org' } }),
}))
vi.mock('../../services/orgService', () => ({
  useGetOrgByName: () => ({ data: { id: 'org-1' }, isLoading: false }),
}))
vi.mock('../../services/providersService', () => ({
  useListProviders: () => ({
    data: [{
      id: 'provider-1',
      name: 'OpenAI',
      available_models: [{ id: 'api-model', enabled: true, type: 'text' }],
    }],
    isLoading: false,
  }),
}))
vi.mock('../account/ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: subscriptionState.claude }),
}))
vi.mock('../../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: subscriptionState.codex }),
}))

function renderPicker(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

describe('CodeAgentConfigPicker', () => {
  beforeEach(() => {
    subscriptionState.claude = []
    subscriptionState.codex = []
  })

  it('offers every supported harness independently of Helix Apps', () => {
    renderPicker(
      <CodeAgentConfigPicker
        trigger="harness"
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding harness' }))
    expect(screen.getByRole('button', { name: 'Zed Agent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Goose' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Codex' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'opencode' })).toBeInTheDocument()
  })

  it('shows subscription versus API usage for Claude Code and Codex', () => {
    renderPicker(
      <CodeAgentConfigPicker
        trigger="model"
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding model' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByRole('radio', { name: 'Subscription' })).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'API usage' })).toBeChecked()

    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.getByRole('radio', { name: 'Subscription' })).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'API usage' })).toBeChecked()
  })

  it('emits a complete Codex subscription config', () => {
    subscriptionState.codex = [{ id: 'codex-subscription' }]
    const onChange = vi.fn()
    renderPicker(
      <CodeAgentConfigPicker
        trigger="model"
        onChange={onChange}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding model' }))
    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.getByRole('radio', { name: 'Subscription' })).toBeChecked()
    fireEvent.click(screen.getByRole('button', { name: /GPT-5.6 Sol.*ChatGPT subscription/ }))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'gpt-5.6-sol',
      provider_ref: undefined,
    }))
  })
})
