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
  if (provider.id && provider.id !== '-' && provider.id === ref) return true
  return provider.name?.toLowerCase() === ref.toLowerCase()
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
  return !required || provider.name?.toLowerCase() === required
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
  return compatible.filter((provider) =>
    harness.provider_refs?.some((ref) => providerEndpointMatchesRef(provider, ref)))
}
