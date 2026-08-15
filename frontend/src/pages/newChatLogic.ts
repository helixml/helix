import {
  TypesCodeAgentOverrides,
  TypesCreateTaskRequest,
  TypesProviderEndpoint,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
  TypesSpecTaskPriority,
} from '../api/api'

export type NewChatTaskMode = 'plan' | 'build'
export type NewChatReasoningEffort = 'none' | 'low' | 'medium' | 'high'

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
  appId,
  mode,
  projectId,
  prompt,
  codeAgentOverrides,
  sandboxResourceOverrides,
  sandboxRuntime,
}: {
  appId?: string
  codeAgentOverrides?: TypesCodeAgentOverrides
  mode: NewChatTaskMode
  projectId: string
  prompt: string
  sandboxResourceOverrides?: TypesSandboxResourceOverrides
  sandboxRuntime?: TypesSandboxRuntime
}): TypesCreateTaskRequest {
  return {
    app_id: appId || undefined,
    auto_start: false,
    just_do_it_mode: mode === 'build',
    priority: TypesSpecTaskPriority.SpecTaskPriorityMedium,
    project_id: projectId,
    prompt,
    ...(codeAgentOverrides && Object.values(codeAgentOverrides).some(Boolean)
      ? { code_agent_overrides: codeAgentOverrides }
      : {}),
    ...(sandboxResourceOverrides
      ? { sandbox_resource_overrides: sandboxResourceOverrides }
      : {}),
    ...(sandboxRuntime ? { sandbox_runtime: sandboxRuntime } : {}),
  }
}
