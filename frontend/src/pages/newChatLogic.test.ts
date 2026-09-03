import { describe, expect, it } from 'vitest'
import {
  buildNewChatTaskRequest,
  chooseNewChatModel,
  chooseProjectChatAgentId,
  modelSupportsReasoningEffort,
  newChatHeading,
  newChatModelStorageKey,
  projectChatAgentStorageKey,
  readNewChatReasoningEffort,
  readNewChatModelSelection,
} from './newChatLogic'
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
  TypesSandboxRuntime,
  TypesProviderEndpointStatus,
} from '../api/api'

describe('new chat project mode', () => {
  it('uses the normal-chat heading without project context', () => {
    expect(newChatHeading()).toBe('What would you like to know?')
  })

  it('uses the project name in task mode', () => {
    expect(newChatHeading('Payments')).toBe('What should we build in Payments?')
  })

  it('creates Plan tasks in backlog so attachments can upload before start', () => {
    expect(buildNewChatTaskRequest({
      codeAgentConfig: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
        credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
        model: 'claude-opus-5',
      },
      mode: 'plan',
      projectId: 'prj_1',
      prompt: 'Add billing',
    })).toEqual({
      code_agent_config: {
        runtime: 'claude_code',
        credential_type: 'subscription',
        model: 'claude-opus-5',
      },
      auto_start: false,
      just_do_it_mode: false,
      priority: 'medium',
      project_id: 'prj_1',
      prompt: 'Add billing',
    })
  })

  it('marks Build tasks to skip planning', () => {
    expect(buildNewChatTaskRequest({
      mode: 'build',
      projectId: 'prj_1',
      prompt: 'Fix the tests',
    }).just_do_it_mode).toBe(true)
  })

  it('passes task execution choices through chat-first creation', () => {
    expect(buildNewChatTaskRequest({
      codeAgentConfig: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
        credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
        model: 'gpt-5.6-sol',
        reasoning_effort: 'high',
        service_tier: 'fast',
      },
      mode: 'build',
      projectId: 'prj_1',
      prompt: 'Fix the tests',
      sandboxResourceOverrides: { vcpus: 8, memory_mb: 16384 },
      sandboxRuntime: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    })).toMatchObject({
      code_agent_config: {
        runtime: 'codex_cli',
        credential_type: 'subscription',
        model: 'gpt-5.6-sol',
        reasoning_effort: 'high',
        service_tier: 'fast',
      },
      sandbox_resource_overrides: { vcpus: 8, memory_mb: 16384 },
      sandbox_runtime: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    })
  })

  it('only exposes effort for a selected model that supports it', () => {
    const providers = [{
      id: 'provider-1',
      name: 'openai',
      available_models: [
        { id: 'gpt-basic', model_info: { supports_reasoning_effort: false } },
        { id: 'gpt-reasoning', model_info: { supports_reasoning_effort: true } },
      ],
    }]

    expect(modelSupportsReasoningEffort(providers, 'provider-1', 'gpt-reasoning')).toBe(true)
    expect(modelSupportsReasoningEffort(providers, 'openai', 'gpt-basic')).toBe(false)
    expect(modelSupportsReasoningEffort(providers, 'another-provider', 'gpt-reasoning')).toBe(false)
  })

  it('falls back to medium for an invalid stored effort', () => {
    expect(readNewChatReasoningEffort('high')).toBe('high')
    expect(readNewChatReasoningEffort('ultra')).toBe('medium')
  })

  it('scopes saved chat models to the user and organization', () => {
    expect(newChatModelStorageKey('user_one', 'org_one')).toBe('helix_chat_model:user_one:org_one')
    expect(newChatModelStorageKey('user_two', 'org_one')).not.toBe(newChatModelStorageKey('user_one', 'org_one'))
    expect(newChatModelStorageKey('user_one', 'org_two')).not.toBe(newChatModelStorageKey('user_one', 'org_one'))
  })

  it('prefers a valid saved chat model over the organization default', () => {
    const providers = [{
      id: 'pe_default',
      name: 'default',
      status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
      available_models: [
        { id: 'configured-model', enabled: true, type: 'chat' },
        { id: 'saved-model', enabled: true, type: 'chat' },
      ],
    }]
    expect(chooseNewChatModel(providers, {
      provider: 'pe_default',
      model: 'saved-model',
      reasoningEffort: 'low',
    }, {
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'pe_default',
      model: 'configured-model',
      reasoning_effort: 'high',
    })).toEqual({
      provider: 'pe_default',
      model: 'saved-model',
      reasoningEffort: 'low',
    })
  })

  it('uses an accessible API-key organization default when no saved selection exists', () => {
    const providers = [{
      id: 'pe_helix',
      name: 'helix',
      status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
      available_models: [{ id: 'helix-model', enabled: true, type: 'chat' }],
    }]

    expect(chooseNewChatModel(providers, undefined, {
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'pe_helix',
      model: 'helix-model',
      reasoning_effort: 'high',
    })).toEqual({
      provider: 'pe_helix',
      model: 'helix-model',
      reasoningEffort: 'high',
    })
  })

  it('does not use a subscription organization default for provider chat', () => {
    const providers = [{
      id: 'pe_helix',
      name: 'helix',
      status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
      available_models: [{ id: 'claude-opus-5', enabled: true, type: 'chat' }],
    }]

    expect(chooseNewChatModel(providers, undefined, {
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
      model: 'claude-opus-5',
      reasoning_effort: 'high',
    })).toBeUndefined()
  })

  it('does not substitute another model when a saved selection is inaccessible', () => {
    const providers = [{
      id: 'pe_available',
      name: 'available',
      status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
      available_models: [{ id: 'available-model', enabled: true, type: 'chat' }],
    }]

    expect(chooseNewChatModel(providers, {
      provider: 'pe_missing',
      model: 'configured-model',
      reasoningEffort: 'medium',
    })).toBeUndefined()
  })

  it('rejects malformed saved chat selections', () => {
    expect(readNewChatModelSelection('{')).toBeUndefined()
    expect(readNewChatModelSelection(JSON.stringify({ provider: 'pe_1', model: 'model' }))).toBeUndefined()
  })

  it('keeps project agent preferences isolated by organization', () => {
    expect(projectChatAgentStorageKey('org_one')).toBe('helix_project_chat_agent:org_one')
    expect(projectChatAgentStorageKey('org_two')).toBe('helix_project_chat_agent:org_two')
  })

  it('restores an eligible remembered agent and rejects stale choices', () => {
    const availableIds = ['app_claude', 'app_codex']
    expect(chooseProjectChatAgentId(availableIds, 'app_codex')).toBe('app_codex')
    expect(chooseProjectChatAgentId(availableIds, 'app_org_worker')).toBe('app_claude')
    expect(chooseProjectChatAgentId([], 'app_codex')).toBe('')
  })
})
