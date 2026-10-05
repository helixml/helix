import {
  TypesCodeAgentExecutionConfig,
  TypesCreateTaskRequest,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
  TypesSpecTaskPriority,
} from '../api/api'

export type NewChatTaskMode = 'plan' | 'build'

const PROJECT_CHAT_AGENT_STORAGE_PREFIX = 'helix_project_chat_agent'
const NEW_CHAT_TASK_MODE_STORAGE_PREFIX = 'helix_project_task_mode'

export function newChatTaskModeStorageKey(
  userId: string,
  orgId: string,
  projectId: string,
): string {
  return `${NEW_CHAT_TASK_MODE_STORAGE_PREFIX}:${userId}:${orgId}:${projectId}`
}

export function readNewChatTaskMode(value: string | null): NewChatTaskMode {
  return value === 'plan' ? 'plan' : 'build'
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
      ? mode === 'plan'
        ? { planning_code_agent_config: codeAgentConfig }
        : { code_agent_config: codeAgentConfig }
      : {}),
    ...(sandboxResourceOverrides
      ? { sandbox_resource_overrides: sandboxResourceOverrides }
      : {}),
    ...(sandboxRuntime ? { sandbox_runtime: sandboxRuntime } : {}),
  }
}
