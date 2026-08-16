import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from '../../api/api'
import CodeAgentConfigPicker from './CodeAgentConfigPicker'

const subscriptionState = vi.hoisted(() => ({ claude: [] as any[], codex: [] as any[] }))
// The picker offers only runtimes the org enabled and this viewer can run, so
// every test needs an allow list. Default: everything available on API keys.
const orgProviderState = vi.hoisted(() => ({ providers: [] as any[] }))

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
const navigate = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ params: { org_id: 'test-org' }, navigate }),
}))
vi.mock('../account/ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: subscriptionState.claude }),
}))
vi.mock('../../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: subscriptionState.codex }),
}))
vi.mock('../../services/codeAgentProvidersService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/codeAgentProvidersService')>()),
  useOrgCodeAgentProviders: () => ({ data: orgProviderState.providers, isLoading: false }),
}))

function renderPicker(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

describe('CodeAgentConfigPicker', () => {
  beforeEach(() => {
    navigate.mockClear()
    subscriptionState.claude = []
    subscriptionState.codex = []
    orgProviderState.providers = [
      TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
      TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
    ].map((runtime) => ({
      runtime,
      enabled: true,
      available: true,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_endpoint_id: 'provider-1',
      supports_subscription: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
        || runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
    }))
  })

  it('offers every supported harness independently of Helix Apps', () => {
    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    expect(screen.getByRole('button', { name: 'Zed Agent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Goose' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Codex' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'opencode' })).toBeInTheDocument()
  })

  it('only offers a credentials choice for Claude Code', () => {
    // Claude Code is the one runtime where a member can plausibly hold both a
    // personal subscription and API access, so the choice is theirs per task.
    // Every other runtime authenticates the single way the org configured.
    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))

    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByRole('radio', { name: 'Claude Subscription' })).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'Anthropic API Key' })).toBeChecked()

    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.queryByRole('radio', { name: 'Claude Subscription' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'opencode' }))
    expect(screen.queryByRole('radio', { name: /Subscription/ })).not.toBeInTheDocument()
  })

  it('enables the Claude subscription option once the viewer connects their own account', () => {
    subscriptionState.claude = [{ id: 'claude-subscription' }]
    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }))
    expect(screen.getByRole('radio', { name: 'Claude Subscription' })).not.toBeDisabled()
  })

  it('offers only runtimes the organization enabled for this viewer', () => {
    // Codex enabled for the org but unavailable to this member (no subscription
    // of their own) must not appear — picking it would fail at run time.
    orgProviderState.providers = orgProviderState.providers.map((provider) =>
      provider.runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI
        ? { ...provider, available: false, unavailable_reason: 'Connect your own subscription to use this agent' }
        : provider)

    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    expect(screen.getByRole('button', { name: 'Claude Code' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Codex' })).not.toBeInTheDocument()
  })

  it('offers only the models of the provider the organization pinned', () => {
    // The org already chose the provider for this runtime, so the picker must
    // not offer models from a provider the runtime is not routed through.
    orgProviderState.providers = orgProviderState.providers.map((provider) => ({
      ...provider,
      provider_endpoint_id: 'provider-elsewhere',
    }))

    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    // provider-1 does have api-model, so an empty list is the filter working.
    // Asserted via the empty state rather than the model string, because the
    // collapsed trigger still renders the currently-selected model's name.
    expect(screen.getByText('No models found')).toBeInTheDocument()
    expect(screen.queryByText('OpenAI')).not.toBeInTheDocument()
  })

  it('auto-selects the only usable configuration', async () => {
    // Nothing to choose, so the surface should not make the user open a picker
    // to confirm the single option.
    orgProviderState.providers = [{
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
      enabled: true,
      available: true,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_endpoint_id: 'provider-1',
      supports_subscription: false,
    }]
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)

    await waitFor(() => expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      model: 'api-model',
    })))
  })

  it('does not auto-select when more than one configuration is usable', async () => {
    const onChange = vi.fn()
    renderPicker(<CodeAgentConfigPicker onChange={onChange} />)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not overwrite a configuration the task already has', async () => {
    orgProviderState.providers = [{
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
      enabled: true,
      available: true,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_endpoint_id: 'provider-1',
      supports_subscription: false,
    }]
    const onChange = vi.fn()
    renderPicker(
      <CodeAgentConfigPicker
        onChange={onChange}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('offers a route to settings when nothing is configured', async () => {
    // An empty popover would leave the user with no idea what to do next.
    orgProviderState.providers = []
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)

    // Unconfigured, so the trigger is the single call to action rather than a
    // harness chip implying something is already selected.
    fireEvent.click(screen.getByRole('button', { name: 'Change coding agent' }))
    // Scoped to the dialog: the trigger button carries the same words.
    const dialog = within(await screen.findByRole('dialog'))
    expect(dialog.getByText('Configure harness')).toBeInTheDocument()
    expect(dialog.getByText(/Claude Code, Codex, OpenCode or Zed/)).toBeInTheDocument()
    // The marks caption the sentence above them, so they must match it.
    for (const harness of ['Claude Code', 'Codex', 'opencode', 'Zed Agent']) {
      expect(dialog.getByRole('img', { name: harness })).toBeInTheDocument()
    }

    fireEvent.click(dialog.getByRole('button', { name: 'Configure' }))
    expect(navigate).toHaveBeenCalledWith('org_providers', { org_id: 'test-org' })
  })

  it('reads as unconfigured rather than defaulting to Zed', () => {
    // The control used to render the default runtime, which looked like Zed was
    // already chosen before the user had picked anything.
    renderPicker(<CodeAgentConfigPicker onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Change coding agent' }))
      .toHaveTextContent('Configure harness')
    expect(screen.queryByText('Zed Agent')).not.toBeInTheDocument()
  })

  it('is a single control showing both the harness and its model', () => {
    // Harness and model used to be two buttons opening the same popover, which
    // read as two independent settings.
    renderPicker(
      <CodeAgentConfigPicker
        onChange={vi.fn()}
        value={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
          credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
          provider_ref: 'provider-1',
          model: 'api-model',
        }}
      />,
    )

    const triggers = screen.getAllByRole('button', { name: 'Change coding agent' })
    expect(triggers).toHaveLength(1)
    expect(triggers[0]).toHaveTextContent('api-model')
    // The mark identifies the harness; spelling it out as well is redundant.
    expect(triggers[0]).not.toHaveTextContent('Zed Agent')
    expect(within(triggers[0]).getByRole('img', { name: 'Zed Agent' })).toBeInTheDocument()
  })
})
