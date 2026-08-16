import {
  TypesCodeAgentRuntime,
  TypesProviderEndpoint,
} from '../api/api'

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
