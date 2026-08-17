import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentOverrides,
  TypesCodeAgentRuntime,
} from '../api/api'
import { AGENT_TYPE_ZED_EXTERNAL, IApp, IAssistantConfig } from '../types'

function codeAssistant(app?: IApp): IAssistantConfig | undefined {
  return app?.config?.helix?.assistants?.find(
    (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL,
  ) || app?.config?.helix?.assistants?.[0]
}

export function codeAgentExecutionConfigFromApp(
  app: IApp | undefined,
  overrides: TypesCodeAgentOverrides = {},
): TypesCodeAgentExecutionConfig | undefined {
  const assistant = codeAssistant(app)
  if (!assistant) return undefined

  const runtime = (assistant.code_agent_runtime || 'zed_agent') as TypesCodeAgentRuntime
  const credentialType = (assistant.code_agent_credential_type || 'api_key') as TypesCodeAgentCredentialType
  const subscription = credentialType === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
  let providerRef = ''
  let model = ''

  if (runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode) {
    if (subscription) {
      model = assistant.claude_subscription_model || 'claude-opus-5'
    } else {
      providerRef = assistant.generation_model_provider || assistant.provider || ''
      model = assistant.generation_model || assistant.model || ''
    }
  } else {
    providerRef = subscription ? '' : assistant.provider || assistant.generation_model_provider || ''
    model = assistant.model || assistant.generation_model || ''
  }

  return {
    runtime,
    credential_type: credentialType,
    provider_ref: subscription ? undefined : overrides.provider_ref || providerRef,
    model: overrides.model || model,
    reasoning_effort: overrides.reasoning_effort || assistant.reasoning_effort,
    service_tier: overrides.service_tier,
    goose_recipe_repo_url: assistant.goose_recipe_repo_url,
    goose_recipes: assistant.goose_recipes,
  }
}

export function findCodeAgentAppForConfig(
  apps: IApp[] | undefined,
  config: TypesCodeAgentExecutionConfig | undefined,
): IApp | undefined {
  if (!apps || !config) return undefined
  return apps.find((app) => {
    const candidate = codeAgentExecutionConfigFromApp(app)
    if (!candidate) return false
    return candidate.runtime === config.runtime
      && candidate.credential_type === config.credential_type
      && (candidate.provider_ref || '') === (config.provider_ref || '')
      && (candidate.model || '') === (config.model || '')
      && (candidate.reasoning_effort || '') === (config.reasoning_effort || '')
      && (candidate.goose_recipe_repo_url || '') === (config.goose_recipe_repo_url || '')
      && JSON.stringify(candidate.goose_recipes || []) === JSON.stringify(config.goose_recipes || [])
  })
}
