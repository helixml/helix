import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'

/**
 * Query key factory for Kodit enrichments
 */
export const koditEnrichmentsQueryKey = (repoId: string) => ['kodit', 'enrichments', repoId]
export const koditStatusQueryKey = (repoId: string) => ['kodit', 'status', repoId]

/**
 * Hook to fetch code intelligence enrichments for a repository
 */
export function useKoditEnrichments(repoId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: koditEnrichmentsQueryKey(repoId),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesEnrichmentsDetail(repoId)
      return response.data
    },
    enabled: options?.enabled !== false && !!repoId,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval: 30 * 1000, // Refetch every 30 seconds for updates
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
 * Helper to group enrichments by type
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
 * Get display name for enrichment type
 */
export function getEnrichmentTypeName(type: string): string {
  const names: Record<string, string> = {
    'architecture': 'Architecture',
    'documentation': 'Documentation',
    'dependencies': 'Dependencies',
    'security': 'Security',
    'performance': 'Performance',
    'testing': 'Testing',
    'other': 'Other',
  }

  return names[type] || type
}

/**
 * Get icon for enrichment type
 */
export function getEnrichmentTypeIcon(type: string): string {
  const icons: Record<string, string> = {
    'architecture': '🏗️',
    'documentation': '📚',
    'dependencies': '📦',
    'security': '🔒',
    'performance': '⚡',
    'testing': '✅',
    'other': '💡',
  }

  return icons[type] || '💡'
}
