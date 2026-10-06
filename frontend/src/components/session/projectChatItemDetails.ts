import { TypesSandboxRuntime } from '../../api/api'
import type { IApp } from '../../types'
import { effectiveSpecTaskSandboxRuntime } from '../../utils/specTaskSandboxRuntime'
import { getAgentHarnessLabel, getAgentHarnessModel, getAgentHarnessRuntime } from '../agent/AgentHarness'
import type { SidebarItem } from './ProjectChatSidebar.logic'
import { DEFAULT_SANDBOX_PRESET } from '../../constants/sandboxPresets'

export type ProjectChatItemDetails = {
  repository?: string
  branch?: string
  /** Raw runtime id, for `AgentHarness` — not a display string. */
  runtime?: string
  harness?: string
  model?: string
  compute?: string
  environment?: 'Headless' | 'Full Desktop'
}

type DetailsInput = {
  item: SidebarItem
  apps: IApp[]
  repository?: string
  branch?: string
}

/**
 * The facts a sidebar row can show about a thread.
 *
 * The hover tooltip (desktop) and the second line of each row (mobile) are the
 * same information at two densities, so they resolve it here. A phone has no
 * hover, so if these two ever disagreed the phone would silently be the wrong
 * one — with no way for anyone to notice.
 */
export const getProjectChatItemDetails = ({
  item,
  apps,
  repository,
  branch,
}: DetailsInput): ProjectChatItemDetails => {
  // A spec task carries its own agent config. A plain chat inherits the
  // configured app's, falling back to whatever the session recorded.
  const configuredAppID = item.kind === 'spec-task' ? undefined : item.session?.app_id
  const configuredApp = configuredAppID
    ? apps.find((app) => app.id === configuredAppID)
    : undefined

  const runtime = item.task?.code_agent_config?.runtime || (configuredApp
    ? getAgentHarnessRuntime(configuredApp)
    : item.session?.metadata?.code_agent_runtime || item.session?.metadata?.agent_type)

  const model = item.task?.code_agent_config?.model || (configuredApp
    ? getAgentHarnessModel(configuredApp)
    : item.session?.model_name)

  const details: ProjectChatItemDetails = {
    repository,
    branch,
    runtime: runtime || undefined,
    harness: runtime ? getAgentHarnessLabel(runtime) : undefined,
    model: model || undefined,
  }

  if (item.kind !== 'spec-task' || !item.task) return details

  const vcpus = item.task.sandbox_resource_overrides?.vcpus || DEFAULT_SANDBOX_PRESET.vcpus
  const memoryMb = item.task.sandbox_resource_overrides?.memory_mb || DEFAULT_SANDBOX_PRESET.memory_mb
  const memory = memoryMb % 1024 === 0
    ? `${memoryMb / 1024} GB RAM`
    : `${memoryMb} MB RAM`

  details.compute = `${vcpus} vCPU · ${memory}`
  details.environment = effectiveSpecTaskSandboxRuntime(item.task.sandbox_runtime)
    === TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu
    ? 'Headless'
    : 'Full Desktop'

  return details
}

/**
 * The branch a row should name: what the task actually works on, else what it
 * branched from, else the project default.
 */
export const resolveProjectChatItemBranch = (
  item: SidebarItem,
  projectDefaultBranch?: string,
): string | undefined =>
  item.task?.branch_name || item.task?.base_branch || projectDefaultBranch || undefined
