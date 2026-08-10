import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { IAppFlatState } from '../../types'
import AppSettings from './AppSettings'

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: undefined }),
}))

vi.mock('../create/AdvancedModelPicker', () => ({
  AdvancedModelPicker: ({ hint, onSelectModel }: { hint: string, onSelectModel: (provider: string, model: string) => void }) => (
    <button onClick={() => onSelectModel(hint.includes('Claude') ? 'anthropic' : 'openai', 'selected-model')}>
      Pick model
    </button>
  ),
}))

vi.mock('../agent', () => ({
  AgentTypeSelector: () => null,
}))

vi.mock('../../hooks/useApi', () => ({
  default: () => ({ getApiClient: () => ({}) }),
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ params: {} }),
}))

vi.mock('../../services/orgService', () => ({
  useGetOrgByName: () => ({ data: undefined }),
}))

vi.mock('../../services/providersService', () => ({
  useListProviders: () => ({ data: [{ name: 'anthropic' }, { name: 'openai' }] }),
}))

vi.mock('../account/ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: [] }),
}))

vi.mock('../../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: [] }),
}))

const renderSettings = (app: IAppFlatState, onUpdate = vi.fn().mockResolvedValue(undefined)) => {
  render(
    <AppSettings
      id=""
      app={{
        default_agent_type: 'zed_external',
        code_agent_credential_type: 'subscription',
        ...app,
      }}
      onUpdate={onUpdate}
      section="runtime"
      hideAgentType
    />,
  )
  return onUpdate
}

const selectRuntime = (name: string) => {
  fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
  fireEvent.click(screen.getByText(name, { selector: '.MuiMenuItem-root *' }))
}

describe('AppSettings API-key model persistence', () => {
  it.each([
    ['claude_code', 'Anthropic API Key', {
      code_agent_runtime: 'claude_code',
      code_agent_credential_type: 'api_key',
      generation_model_provider: 'anthropic',
      generation_model: 'selected-model',
    }],
    ['codex_cli', 'OpenAI API Key', {
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'api_key',
      provider: 'openai',
      model: 'selected-model',
    }],
  ] as const)('waits for a complete %s model before saving API-key mode', (runtime, label, expected) => {
    const onUpdate = renderSettings({ code_agent_runtime: runtime })

    fireEvent.click(screen.getByLabelText(label))
    expect(onUpdate).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Pick model' }))
    expect(onUpdate).toHaveBeenCalledWith(expected)
  })

  it.each([
    ['claude_code', 'Anthropic API Key', {
      generation_model_provider: 'anthropic',
      generation_model: 'claude-existing',
    }, {
      code_agent_runtime: 'claude_code',
      code_agent_credential_type: 'api_key',
      generation_model_provider: 'anthropic',
      generation_model: 'claude-existing',
    }],
    ['codex_cli', 'OpenAI API Key', {
      provider: 'openai',
      model: 'gpt-5.6-sol',
    }, {
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'api_key',
      provider: 'openai',
      model: 'gpt-5.6-sol',
    }],
  ] as const)('saves %s mode with its existing model atomically', (runtime, label, selection, expected) => {
    const onUpdate = renderSettings({ code_agent_runtime: runtime, ...selection })

    fireEvent.click(screen.getByLabelText(label))

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenCalledWith(expected)
  })

  it.each([
    ['codex_cli', 'claude_code', 'Claude Code', {
      provider: 'openai',
      model: 'gpt-5.6-sol',
      generation_model_provider: 'anthropic',
      generation_model: 'claude-opus-4-8',
    }, {
      code_agent_runtime: 'claude_code',
      code_agent_credential_type: 'api_key',
      generation_model_provider: 'anthropic',
      generation_model: 'claude-opus-4-8',
    }],
    ['claude_code', 'codex_cli', 'Codex', {
      provider: 'openai',
      model: 'gpt-5.6-sol',
      generation_model_provider: 'anthropic',
      generation_model: 'claude-opus-4-8',
    }, {
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'api_key',
      provider: 'openai',
      model: 'gpt-5.6-sol',
    }],
  ] as const)('saves %s to %s with the target selection', (runtime, _target, label, selection, expected) => {
    const onUpdate = renderSettings({
      code_agent_runtime: runtime,
      code_agent_credential_type: 'api_key',
      ...selection,
    })

    selectRuntime(label)

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenCalledWith(expected)
  })

  it('defers a runtime switch when the target subscription is unavailable', () => {
    const onUpdate = renderSettings({
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'subscription',
      model: 'gpt-5.6-sol',
    })

    selectRuntime('Claude Code')

    expect(onUpdate).not.toHaveBeenCalled()
  })
})
