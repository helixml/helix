import { useMemo } from 'react'
import {
  TypesCodeAgentModelOption,
  TypesCodeAgentRuntime,
  TypesServerConfigForFrontend,
} from '../api/api'
import useAccount from './useAccount'

export interface CodeAgentModelChoice {
  id: string
  label: string
}

export interface CodeAgentModelCatalogue {
  claudeModels: CodeAgentModelChoice[]
  codexModels: CodeAgentModelChoice[]
  defaultClaudeModel: string
  defaultCodexModel: string
  /** Current-generation model ids per harness, best first. */
  currentModels: (runtime: TypesCodeAgentRuntime) => string[]
  /** Changes only when the served list does; use it in dependency arrays. */
  version: string
}

function options(
  config: TypesServerConfigForFrontend | undefined,
  runtime: TypesCodeAgentRuntime,
): TypesCodeAgentModelOption[] {
  return config?.code_agent_models?.[runtime] ?? []
}

// "Claude Opus 5.5 (1M context)" → "Claude Opus 5.5 (1M context, recommended)".
function recommended(name: string): string {
  return name.endsWith(')') ? `${name.slice(0, -1)}, recommended)` : `${name} (recommended)`
}

function choices(models: TypesCodeAgentModelOption[]): CodeAgentModelChoice[] {
  return models.map((m, i) => {
    const name = m.name || m.id || ''
    return { id: m.id || '', label: i === 0 ? recommended(name) : name }
  })
}

/**
 * The vendor model catalogue the backend serves on /api/v1/config
 * (types.CodeAgentModels), best first. It is the only place the list is
 * maintained: the first entry is each harness's default, so a newer model put
 * at the top of the backend list reaches every picker and default here.
 */
export function codeAgentModelCatalogue(
  config: TypesServerConfigForFrontend | undefined,
): CodeAgentModelCatalogue {
  const claude = options(config, TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode)
  const codex = options(config, TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI)
  return {
    version: JSON.stringify(config?.code_agent_models ?? {}),
    claudeModels: choices(claude),
    codexModels: choices(codex),
    defaultClaudeModel: claude[0]?.id || '',
    defaultCodexModel: codex[0]?.id || '',
    currentModels: (runtime) =>
      options(config, runtime).filter((m) => m.current).map((m) => m.id || ''),
  }
}

// Memoised on the serialised list (a primitive), so the catalogue and its
// arrays keep their identity across renders until the served list changes.
export function useCodeAgentModelCatalogue(): CodeAgentModelCatalogue {
  const config = useAccount().serverConfig
  const version = JSON.stringify(config?.code_agent_models ?? {})
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => codeAgentModelCatalogue(config), [version])
}
