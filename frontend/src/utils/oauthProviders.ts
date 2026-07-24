import { TypesOAuthConnection, TypesOAuthProvider, TypesExternalRepositoryType } from '../api/api'

// Provider type constants to avoid magic strings
export const PROVIDER_TYPES = {
  GITHUB: 'github',
  GITLAB: 'gitlab',
  AZURE_DEVOPS: 'azure-devops',
  ADO: 'ado', // Azure DevOps alternate
  BITBUCKET: 'bitbucket',
  CUSTOM: 'custom',
} as const

export type ProviderType = 'github' | 'gitlab' | 'azure-devops' | 'bitbucket'

/**
 * Check if a provider type or name matches a target provider.
 * Handles various naming conventions and partial matches.
 */
export const matchesProviderType = (
  connType: string | undefined | null,
  connName: string | undefined | null,
  targetProvider: ProviderType
): boolean => {
  const type = connType?.toLowerCase()
  const name = connName?.toLowerCase()

  switch (targetProvider) {
    case 'azure-devops':
      return type === 'azure-devops' || type === 'ado' ||
             name?.includes('azure') || name?.includes('ado') || false
    case 'bitbucket':
      return type === 'bitbucket' || name?.includes('bitbucket') || false
    case 'github':
      return type === 'github' || name === 'github' || name?.includes('github') || false
    case 'gitlab':
      return type === 'gitlab' || name === 'gitlab' || name?.includes('gitlab') || false
    default:
      return type === targetProvider || name === targetProvider || name?.includes(targetProvider) || false
  }
}

/**
 * Find an OAuth connection that matches the given provider type.
 * Checks both type and name since providers may be configured with type="custom".
 */
export const findOAuthConnectionForProvider = (
  connections: TypesOAuthConnection[] | undefined,
  providerType: ProviderType
): TypesOAuthConnection | undefined => {
  return connections?.find(conn =>
    matchesProviderType(conn.provider?.type, conn.provider?.name, providerType)
  )
}

/**
 * Find an OAuth provider that matches the given provider type.
 * Only returns enabled providers (or providers without explicit enabled=false).
 */
export const findOAuthProviderForType = (
  providers: TypesOAuthProvider[] | undefined,
  providerType: ProviderType
): TypesOAuthProvider | undefined => {
  return providers?.find(p => {
    if (p.enabled === false) return false
    return matchesProviderType(p.type, p.name, providerType)
  })
}

/**
 * Map frontend provider type to API external repository type.
 */
export const mapProviderToRepoType = (provider: ProviderType): TypesExternalRepositoryType => {
  switch (provider) {
    case 'github':
      return TypesExternalRepositoryType.ExternalRepositoryTypeGitHub
    case 'gitlab':
      return TypesExternalRepositoryType.ExternalRepositoryTypeGitLab
    case 'azure-devops':
      return TypesExternalRepositoryType.ExternalRepositoryTypeADO
    case 'bitbucket':
      return TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket
  }
}

// Full connect-time scope set for GitHub VCS. GitHub REPLACES (does not union) a
// token's scopes on re-authorization, so every connect/reconnect entry point must
// request the whole set or it silently downgrades an existing grant. Single source
// of truth — re-exported from hooks/useOAuthFlow for existing importers.
export const GITHUB_VCS_SCOPES = ['repo', 'workflow', 'read:org', 'read:user', 'user:email']

/**
 * Scopes to request when connecting/reconnecting a VCS provider, by provider
 * type/name. Returns undefined for non-VCS (or unknown) providers so their default
 * connect behaviour is left untouched. Mirrors the backend vcs.RequiredScopes plus
 * GitHub's workflow scope, and matches BrowseProvidersDialog.
 */
export const vcsScopesForProvider = (
  type: string | undefined | null,
  name: string | undefined | null,
): string[] | undefined => {
  if (matchesProviderType(type, name, 'github')) return GITHUB_VCS_SCOPES
  if (matchesProviderType(type, name, 'gitlab')) return ['api', 'read_repository', 'write_repository', 'read_user']
  if (matchesProviderType(type, name, 'azure-devops')) return ['vso.code_write']
  if (matchesProviderType(type, name, 'bitbucket')) return ['repository', 'repository:write']
  return undefined
}

/**
 * Check if an OAuth connection has the required scopes.
 * Returns false if no scopes are present on the connection or if requiredScopes is empty.
 */
export const hasRequiredScopes = (
  connectionScopes: string[] | undefined,
  requiredScopes: string[]
): boolean => {
  // If no scopes required, return false (caller should specify what they need)
  if (!requiredScopes || requiredScopes.length === 0) return false
  // If connection has no scopes, it doesn't have the required ones
  if (!connectionScopes || connectionScopes.length === 0) return false

  return requiredScopes.every(required =>
    connectionScopes.some(scope => scope === required || scope.startsWith(required + ':'))
  )
}
