import { TypesCodeAgentRuntime, TypesProviderEndpoint } from '../api/api'

type NativeProvider = 'anthropic' | 'openai'

// The current-model lists themselves come from the backend catalogue
// (useCodeAgentModelCatalogue). This file only maps runtimes and provider
// endpoints onto the vendor whose list applies.
const RUNTIME_FOR_PROVIDER: Record<NativeProvider, TypesCodeAgentRuntime> = {
  anthropic: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  openai: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
}

export function nativeProviderForRuntime(runtime: TypesCodeAgentRuntime): NativeProvider | undefined {
  if (runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode) return 'anthropic'
  if (runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI) return 'openai'
  return undefined
}

export function nativeProviderForEndpoint(provider: TypesProviderEndpoint): NativeProvider | undefined {
  const name = provider.name?.toLowerCase()
  if (name === 'anthropic' || provider.base_url?.startsWith('https://api.anthropic.com/')) return 'anthropic'
  if (name === 'openai' || provider.base_url?.startsWith('https://api.openai.com/')) return 'openai'
  return undefined
}

/** The runtime whose catalogue lists a native provider's current models. */
export function runtimeForNativeProvider(provider: NativeProvider): TypesCodeAgentRuntime {
  return RUNTIME_FOR_PROVIDER[provider]
}

/**
 * Whether a native vendor model is older than the vendor's current set.
 * currentIds is undefined for non-native providers, which are never legacy.
 */
export function isLegacyNativeModel(modelID: string, currentIds: string[] | undefined): boolean {
  if (!currentIds || currentIds.length === 0) return false
  const normalized = (modelID.split('/').pop() || modelID).toLowerCase()
  return !currentIds.some((id) => normalized === id || normalized.startsWith(`${id}-`))
}
