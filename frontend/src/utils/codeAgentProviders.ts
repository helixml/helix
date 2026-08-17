import {
  TypesCodeAgentRuntime,
  TypesProviderEndpoint,
  TypesProviderEndpointStatus,
} from '../api/api'

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
