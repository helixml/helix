import { TypesCodeAgentRuntime, TypesProviderEndpoint } from '../api/api'

type NativeProvider = 'anthropic' | 'openai'

const CURRENT_NATIVE_MODELS: Record<NativeProvider, string[]> = {
  anthropic: ['claude-opus-5', 'claude-fable-5'],
  openai: ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna'],
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

export function currentNativeModels(provider: NativeProvider | undefined): string[] {
  return provider ? CURRENT_NATIVE_MODELS[provider] : []
}

export function isLegacyNativeModel(modelID: string, provider: NativeProvider | undefined): boolean {
  if (!provider) return false
  const normalized = (modelID.split('/').pop() || modelID).toLowerCase()
  return !CURRENT_NATIVE_MODELS[provider].some((id) =>
    normalized === id || normalized.startsWith(`${id}-`))
}
