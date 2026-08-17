import { describe, expect, it } from 'vitest'
import { TypesCodeAgentCredentialType, TypesCodeAgentRuntime } from '../api/api'
import { IApp } from '../types'
import { codeAgentExecutionConfigFromApp, findCodeAgentAppForConfig } from './codeAgentExecutionConfig'

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
