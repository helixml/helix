import { describe, expect, it } from 'vitest'
import { TypesCodeAgentCredentialType, TypesCodeAgentRuntime } from '../api/api'
import { IApp } from '../types'
import {
  codeAgentExecutionConfigFromApp,
  findCodeAgentAppForConfig,
  shouldSeedProjectCodeAgentConfig,
} from './codeAgentExecutionConfig'

function appWithAssistant(assistant: IApp['config']['helix']['assistants'][number]): IApp {
  return {
    id: 'app-1',
    agent_kind: 'coding_agent',
    config: { helix: { name: 'Coding', description: '', external_url: '', assistants: [assistant] }, secrets: {}, allowed_domains: [] },
    global: false,
    created: new Date(),
    updated: new Date(),
    owner: 'user-1',
    owner_type: 'user',
  }
}

describe('codeAgentExecutionConfigFromApp', () => {
  it('materializes API-key runtime and applies task selections', () => {
    const config = codeAgentExecutionConfigFromApp(appWithAssistant({
      agent_type: 'zed_external',
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'api_key',
      provider: 'provider-old',
      model: 'model-old',
    }), {
      provider_ref: 'provider-new',
      model: 'gpt-5.6-sol',
      reasoning_effort: 'xhigh',
      service_tier: 'fast',
    })

    expect(config).toMatchObject({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'provider-new',
      model: 'gpt-5.6-sol',
      reasoning_effort: 'xhigh',
      service_tier: 'fast',
    })
  })

  it('does not persist a provider for subscription agents', () => {
    const config = codeAgentExecutionConfigFromApp(appWithAssistant({
      agent_type: 'zed_external',
      code_agent_runtime: 'claude_code',
      code_agent_credential_type: 'subscription',
      claude_subscription_model: 'claude-opus-5',
    }), { provider_ref: 'must-not-survive' })

    expect(config?.provider_ref).toBeUndefined()
    expect(config?.model).toBe('claude-opus-5')
  })

  it('maps a materialized project config back to its selector app', () => {
    const app = appWithAssistant({
      agent_type: 'zed_external',
      code_agent_runtime: 'claude_code',
      code_agent_credential_type: 'subscription',
      claude_subscription_model: 'claude-opus-5',
    })
    const config = codeAgentExecutionConfigFromApp(app)

    expect(findCodeAgentAppForConfig([app], config)?.id).toBe(app.id)
  })
})

describe('shouldSeedProjectCodeAgentConfig', () => {
  const pick = {
    runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
    credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
    model: 'claude-opus-5',
  }

  it('seeds a project that has no coding configuration', () => {
    expect(shouldSeedProjectCodeAgentConfig(undefined, pick)).toBe(true)
    expect(shouldSeedProjectCodeAgentConfig({}, pick)).toBe(true)
  })

  it('leaves a project that already records a model alone', () => {
    expect(shouldSeedProjectCodeAgentConfig({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
      provider_ref: 'openai',
      model: 'gpt-5.6-sol',
    }, pick)).toBe(false)
  })

  it('completes a runtime-only project default with a model for that runtime', () => {
    const deferred = {
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
    }
    expect(shouldSeedProjectCodeAgentConfig(deferred, pick)).toBe(true)
    expect(shouldSeedProjectCodeAgentConfig(deferred, {
      ...pick,
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
    })).toBe(false)
  })

  it('ignores an incomplete pick', () => {
    expect(shouldSeedProjectCodeAgentConfig(undefined, undefined)).toBe(false)
    expect(shouldSeedProjectCodeAgentConfig(undefined, { runtime: pick.runtime })).toBe(false)
  })
})
