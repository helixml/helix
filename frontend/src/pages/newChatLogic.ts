import {
  TypesCodeAgentExecutionConfig,
  TypesCreateTaskRequest,
  TypesProviderEndpoint,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
  TypesSpecTaskPriority,
} from '../api/api'
import {
  providerEndpointIsConnected,
  resolveProviderEndpointRef,
} from '../utils/codeAgentProviders'

export type NewChatTaskMode = 'plan' | 'build'
export type NewChatReasoningEffort = 'none' | 'low' | 'medium' | 'high'

export interface NewChatModelSelection {
  provider: string
  model: string
  reasoningEffort: NewChatReasoningEffort
}

// Mirrors the tiers agent settings offers and types.ValidReasoningEffort accepts.
export const NEW_CHAT_REASONING_EFFORT_OPTIONS: ReadonlyArray<{
  value: NewChatReasoningEffort
  label: string
}> = [
  { value: 'none', label: 'Off' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
]

const PROJECT_CHAT_AGENT_STORAGE_PREFIX = 'helix_project_chat_agent'
const NEW_CHAT_MODEL_STORAGE_PREFIX = 'helix_chat_model'

export function newChatModelStorageKey(userId: string, orgId: string): string {
  return `${NEW_CHAT_MODEL_STORAGE_PREFIX}:${userId}:${orgId}`
}

export function readNewChatModelSelection(value: string | null): NewChatModelSelection | undefined {
  if (!value) return undefined
  try {
    const selection = JSON.parse(value) as Partial<NewChatModelSelection>
    if (typeof selection.provider !== 'string' || typeof selection.model !== 'string') return undefined
    if (!NEW_CHAT_REASONING_EFFORT_OPTIONS.some((option) => option.value === selection.reasoningEffort)) return undefined
    return selection as NewChatModelSelection
  } catch {
    return undefined
  }
}

export function chooseNewChatModel(
  providers: TypesProviderEndpoint[],
  saved: NewChatModelSelection | undefined,
  orgDefault?: TypesCodeAgentExecutionConfig,
): NewChatModelSelection | undefined {
  const resolve = (selection: NewChatModelSelection): NewChatModelSelection | undefined => {
    const provider = resolveProviderEndpointRef(providers, selection.provider)
    const model = provider?.available_models?.find((candidate) =>
      candidate.id === selection.model
      && candidate.enabled
      && (!candidate.type || candidate.type === 'chat' || candidate.type === 'text'))
    if (!provider || !providerEndpointIsConnected(provider) || !model) return undefined
    return {
      ...selection,
      provider: provider.id && provider.id !== '-' ? provider.id : provider.name || '',
    }
  }

  const savedSelection = saved && resolve(saved)
  if (savedSelection) return savedSelection
  if (orgDefault?.credential_type !== 'api_key' || !orgDefault.provider_ref || !orgDefault.model) {
    return undefined
  }
  return resolve({
    provider: orgDefault.provider_ref,
    model: orgDefault.model,
    reasoningEffort: readNewChatReasoningEffort(orgDefault.reasoning_effort || null),
  })
}

export function parseOrgDefaultRuntime(value?: string): TypesCodeAgentExecutionConfig | undefined {
  if (!value) return undefined
  try {
    const config = JSON.parse(value)
    if (!config.code_agent_runtime || !config.code_agent_credential_type || !config.model) return undefined
    return {
      runtime: config.code_agent_runtime,
      credential_type: config.code_agent_credential_type,
      provider_ref: config.provider || undefined,
      model: config.model,
      reasoning_effort: config.reasoning_effort || 'none',
    }
  } catch {
    return undefined
  }
}

export function projectChatAgentStorageKey(orgId: string): string {
  return `${PROJECT_CHAT_AGENT_STORAGE_PREFIX}:${orgId}`
}

export function chooseProjectChatAgentId(
  availableIds: string[],
  rememberedId: string | null,
): string {
  return rememberedId && availableIds.includes(rememberedId)
    ? rememberedId
    : availableIds[0] || ''
}

export function readNewChatReasoningEffort(value: string | null): NewChatReasoningEffort {
  return NEW_CHAT_REASONING_EFFORT_OPTIONS.some((option) => option.value === value)
    ? value as NewChatReasoningEffort
    : 'medium'
}

export function modelSupportsReasoningEffort(
  providers: TypesProviderEndpoint[],
  providerRef: string,
  modelId: string,
): boolean {
  if (!modelId) return false
  return providers.some((provider) => {
    const matchesProvider = !providerRef
      || provider.id === providerRef
      || provider.name?.toLowerCase() === providerRef.toLowerCase()
    if (!matchesProvider) return false
    return provider.available_models?.some(
      (model) => model.id === modelId && model.model_info?.supports_reasoning_effort,
    ) || false
  })
}

export function newChatHeading(projectName?: string): string {
  return projectName
    ? `What should we build in ${projectName}?`
    : 'What would you like to know?'
}

export function buildNewChatTaskRequest({
  mode,
  projectId,
  prompt,
  codeAgentConfig,
  sandboxResourceOverrides,
  sandboxRuntime,
}: {
  codeAgentConfig?: TypesCodeAgentExecutionConfig
  mode: NewChatTaskMode
  projectId: string
  prompt: string
  sandboxResourceOverrides?: TypesSandboxResourceOverrides
  sandboxRuntime?: TypesSandboxRuntime
}): TypesCreateTaskRequest {
  return {
    auto_start: false,
    just_do_it_mode: mode === 'build',
    priority: TypesSpecTaskPriority.SpecTaskPriorityMedium,
    project_id: projectId,
    prompt,
    ...(codeAgentConfig
      ? { code_agent_config: codeAgentConfig }
      : {}),
    ...(sandboxResourceOverrides
      ? { sandbox_resource_overrides: sandboxResourceOverrides }
      : {}),
    ...(sandboxRuntime ? { sandbox_runtime: sandboxRuntime } : {}),
  }
}
