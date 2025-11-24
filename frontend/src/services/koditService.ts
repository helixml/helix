import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'

/**
 * Kodit enrichment type constants (high-level categories)
 */
export const KODIT_TYPE_USAGE = 'usage' // How to use the code
export const KODIT_TYPE_DEVELOPER = 'developer' // Development documentation
export const KODIT_TYPE_LIVING_DOCUMENTATION = 'living_documentation' // Dynamic documentation

/**
 * Kodit enrichment subtype constants (specific types)
 */
// Usage subtypes
export const KODIT_SUBTYPE_SNIPPET = 'snippet' // Code snippets
export const KODIT_SUBTYPE_EXAMPLE = 'example' // Full examples
export const KODIT_SUBTYPE_COOKBOOK = 'cookbook' // How-to guides

// Developer subtypes
export const KODIT_SUBTYPE_ARCHITECTURE = 'architecture' // Architecture docs
export const KODIT_SUBTYPE_API_DOCS = 'api_docs' // API documentation
export const KODIT_SUBTYPE_DATABASE_SCHEMA = 'database_schema' // Database schemas

// Living documentation subtypes
export const KODIT_SUBTYPE_COMMIT_DESCRIPTION = 'commit_description' // Commit descriptions

/**
 * Query key factory for Kodit enrichments
 */
export const koditEnrichmentsQueryKey = (repoId: string, commitSha?: string) =>
  commitSha ? ['kodit', 'enrichments', repoId, commitSha] : ['kodit', 'enrichments', repoId]
export const koditEnrichmentDetailQueryKey = (repoId: string, enrichmentId: string) =>
  ['kodit', 'enrichments', repoId, enrichmentId]
export const koditCommitsQueryKey = (repoId: string) => ['kodit', 'commits', repoId]
export const koditStatusQueryKey = (repoId: string) => ['kodit', 'status', repoId]

/**
 * Hook to fetch code intelligence enrichments for a repository
 */
export function useKoditEnrichments(repoId: string, commitSha?: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: koditEnrichmentsQueryKey(repoId, commitSha),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesEnrichmentsDetail(repoId, {
        commit_sha: commitSha,
      })
      return response.data
    },
    enabled: options?.enabled !== false && !!repoId,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval: commitSha ? undefined : 30 * 1000, // Only auto-refetch for latest, not for specific commits
  })
}

/**
 * Hook to fetch a single enrichment with full content
 */
export function useKoditEnrichmentDetail(
  repoId: string,
  enrichmentId: string,
  options?: { enabled?: boolean }
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: koditEnrichmentDetailQueryKey(repoId, enrichmentId),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesEnrichmentsDetail2(
        repoId,
        enrichmentId
      )
      return response.data
    },
    enabled: options?.enabled !== false && !!repoId && !!enrichmentId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch commits from Kodit
 */
export function useKoditCommits(repoId: string, limit?: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: koditCommitsQueryKey(repoId),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesKoditCommitsDetail(repoId, {
        limit,
      })
      return response.data
    },
    enabled: options?.enabled !== false && !!repoId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch Kodit indexing status for a repository
 */
export function useKoditStatus(repoId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: koditStatusQueryKey(repoId),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesKoditStatusDetail(repoId)
      return response.data
    },
    enabled: options?.enabled !== false && !!repoId,
    staleTime: 10 * 1000, // 10 seconds
    refetchInterval: 10 * 1000, // Refetch every 10 seconds for status updates
  })
}

/**
 * Helper to group enrichments by subtype (the specific enrichment type)
 * Use subtype as the primary grouping key since that's what users care about
 */
export function groupEnrichmentsBySubtype(enrichments: any[]) {
  const groups: Record<string, any[]> = {}

  for (const enrichment of enrichments) {
    const subtype = enrichment.attributes?.subtype || 'other'
    if (!groups[subtype]) {
      groups[subtype] = []
    }
    groups[subtype].push(enrichment)
  }

  return groups
}

/**
 * Helper to group enrichments by high-level type
 */
export function groupEnrichmentsByType(enrichments: any[]) {
  const groups: Record<string, any[]> = {}

  for (const enrichment of enrichments) {
    const type = enrichment.attributes?.type || 'other'
    if (!groups[type]) {
      groups[type] = []
    }
    groups[type].push(enrichment)
  }

  return groups
}

/**
 * Get display name for enrichment subtype
 */
export function getEnrichmentSubtypeName(subtype: string): string {
  const names: Record<string, string> = {
    // Usage subtypes
    [KODIT_SUBTYPE_SNIPPET]: 'Code Snippets',
    [KODIT_SUBTYPE_EXAMPLE]: 'Examples',
    [KODIT_SUBTYPE_COOKBOOK]: 'Cookbook',

    // Developer subtypes
    [KODIT_SUBTYPE_ARCHITECTURE]: 'Architecture',
    [KODIT_SUBTYPE_API_DOCS]: 'API Documentation',
    [KODIT_SUBTYPE_DATABASE_SCHEMA]: 'Database Schema',

    // Living documentation subtypes
    [KODIT_SUBTYPE_COMMIT_DESCRIPTION]: 'Recent Changes',

    'other': 'Other',
  }

  return names[subtype] || subtype
}

/**
 * Get display name for high-level enrichment type
 */
export function getEnrichmentTypeName(type: string): string {
  const names: Record<string, string> = {
    [KODIT_TYPE_USAGE]: 'Usage',
    [KODIT_TYPE_DEVELOPER]: 'Developer',
    [KODIT_TYPE_LIVING_DOCUMENTATION]: 'Living Documentation',
    'other': 'Other',
  }

  return names[type] || type
}

/**
 * Get icon for enrichment subtype
 */
export function getEnrichmentSubtypeIcon(subtype: string): string {
  const icons: Record<string, string> = {
    // Usage subtypes
    [KODIT_SUBTYPE_SNIPPET]: '📝',
    [KODIT_SUBTYPE_EXAMPLE]: '💡',
    [KODIT_SUBTYPE_COOKBOOK]: '📖',

    // Developer subtypes
    [KODIT_SUBTYPE_ARCHITECTURE]: '🏗️',
    [KODIT_SUBTYPE_API_DOCS]: '📚',
    [KODIT_SUBTYPE_DATABASE_SCHEMA]: '🗄️',

    // Living documentation subtypes
    [KODIT_SUBTYPE_COMMIT_DESCRIPTION]: '📝',

    'other': '💡',
  }

  return icons[subtype] || '💡'
}

/**
 * Get icon for high-level enrichment type
 */
export function getEnrichmentTypeIcon(type: string): string {
  const icons: Record<string, string> = {
    [KODIT_TYPE_USAGE]: '🎯',
    [KODIT_TYPE_DEVELOPER]: '👨‍💻',
    [KODIT_TYPE_LIVING_DOCUMENTATION]: '📄',
    'other': '💡',
  }

  return icons[type] || '💡'
}
