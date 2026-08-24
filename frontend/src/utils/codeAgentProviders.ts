import {
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
  TypesProviderEndpoint,
  TypesProviderEndpointStatus,
} from '../api/api'

export function providerEndpointMatchesRef(
  provider: TypesProviderEndpoint | undefined,
  ref: string | undefined,
): boolean {
  if (!provider || !ref) return false
  return !!provider.id && provider.id === ref
}

export function canonicalProviderName(name?: string): string {
  const normalized = name?.toLowerCase() || ''
  const legacyName = normalized.startsWith('user/') || normalized.startsWith('global/')
    ? normalized.slice(normalized.indexOf('/') + 1)
    : normalized
  return ['openai', 'togetherai', 'anthropic', 'helix', 'vllm'].includes(legacyName)
    ? legacyName
    : normalized
}

export function resolveProviderEndpointRef(
  providers: TypesProviderEndpoint[],
  ref?: string,
): TypesProviderEndpoint | undefined {
  if (!ref) return undefined
  const exact = providers.find((provider) => providerEndpointMatchesRef(provider, ref))
  if (exact) return exact
  if (ref.startsWith('pe_') || ref.startsWith('global/')) return undefined
  return providers
    .filter((provider) => canonicalProviderName(provider.name) === canonicalProviderName(ref))
    .sort((a, b) => providerPrecedence(b) - providerPrecedence(a))[0]
}

function providerPrecedence(provider: TypesProviderEndpoint): number {
  if (provider.endpoint_type === 'org' || provider.endpoint_type === 'user') return 3
  return provider.id?.startsWith('pe_') ? 2 : 1
}

export function providerEndpointIsConnected(provider: TypesProviderEndpoint): boolean {
  return provider.status === TypesProviderEndpointStatus.ProviderEndpointStatusOK
    && (provider.available_models || []).some((model) => model.enabled
      && (!model.type || model.type === 'chat' || model.type === 'text'))
}

export function requiredProviderNameForRuntime(
  runtime?: TypesCodeAgentRuntime | string,
): string | undefined {
  switch (runtime) {
    case TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode:
      return 'anthropic'
    case TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI:
      return 'openai'
    default:
      return undefined
  }
}

export function providerSupportsCodeAgentRuntime(
  provider: TypesProviderEndpoint,
  runtime?: TypesCodeAgentRuntime | string,
): boolean {
  const required = requiredProviderNameForRuntime(runtime)
  return !required || canonicalProviderName(provider.name) === required
}

export function providersForCodeAgentRuntime(
  providers: TypesProviderEndpoint[],
  runtime?: TypesCodeAgentRuntime | string,
): TypesProviderEndpoint[] {
  return providers.filter((provider) => providerSupportsCodeAgentRuntime(provider, runtime))
}

export function providersForCodeAgentHarness(
  providers: TypesProviderEndpoint[],
  harness: TypesOrgCodeAgentHarnessStatus | undefined,
  runtime?: TypesCodeAgentRuntime | string,
): TypesProviderEndpoint[] {
  if (!harness?.enabled || harness.subscription_enabled === true) return []
  const compatible = providersForCodeAgentRuntime(providers, runtime)
    .filter(providerEndpointIsConnected)
  if (harness.provider_refs == null) return compatible
  const allowed = new Set(harness.provider_refs
    .map((ref) => resolveProviderEndpointRef(compatible, ref)?.id)
    .filter(Boolean))
  return compatible.filter((provider) => provider.id && allowed.has(provider.id))
}
