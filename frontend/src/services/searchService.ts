/**
 * Search Service - Unified search across Helix entities
 *
 * Provides React Query hooks for searching projects, tasks, sessions, and prompts
 */

import { useQuery, UseQueryOptions } from '@tanstack/react-query'
import {
  TypesUnifiedSearchResponse,
  TypesUnifiedSearchResult,
  Api,
} from '../api/api'
import { useApi } from '../hooks/useApi'

// Query key factory
export const searchQueryKey = (query: string, types?: string[], limit?: number) =>
  ['unified-search', query, types?.join(',') || 'all', limit] as const

// Search entity types
export type SearchEntityType = 'projects' | 'tasks' | 'sessions' | 'prompts' | 'code'

// Search request options
export interface UnifiedSearchOptions {
  query: string
  types?: SearchEntityType[]
  limit?: number
  orgId?: string
  enabled?: boolean
}

/**
 * Perform unified search across Helix entities
 */
export async function unifiedSearch(
  apiClient: Api<unknown>['api'],
  options: Omit<UnifiedSearchOptions, 'enabled'>
): Promise<TypesUnifiedSearchResponse> {
  const response = await apiClient.v1SearchList({
    q: options.query,
    types: options.types,
    limit: options.limit,
    org_id: options.orgId,
  })
  return response.data
}

/**
 * React Query hook for unified search
 *
 * Searches across projects, tasks, sessions, and prompts with debouncing
 *
 * @example
 * const { data, isLoading } = useUnifiedSearch({
 *   query: 'authentication',
 *   types: ['tasks', 'prompts'],
 *   limit: 10,
 *   enabled: searchQuery.length >= 2,
 * })
 */
export function useUnifiedSearch(
  options: UnifiedSearchOptions,
  queryOptions?: Omit<UseQueryOptions<TypesUnifiedSearchResponse>, 'queryKey' | 'queryFn'>
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: searchQueryKey(options.query, options.types, options.limit),
    queryFn: () => unifiedSearch(apiClient, options),
    enabled: options.enabled !== false && options.query.length >= 2,
    staleTime: 30 * 1000, // 30 seconds
    ...queryOptions,
  })
}

/**
 * Group search results by type
 */
export function groupResultsByType(results: TypesUnifiedSearchResult[]): Record<string, TypesUnifiedSearchResult[]> {
  return results.reduce((acc, result) => {
    const type = result.type || 'unknown'
    if (!acc[type]) {
      acc[type] = []
    }
    acc[type].push(result)
    return acc
  }, {} as Record<string, TypesUnifiedSearchResult[]>)
}

/**
 * Get icon for search result type
 */
export function getSearchResultIcon(type: string): string {
  switch (type) {
    case 'project':
      return 'folder'
    case 'task':
      return 'task'
    case 'session':
      return 'chat'
    case 'prompt':
      return 'prompt'
    case 'code':
      return 'code'
    default:
      return 'search'
  }
}

/**
 * Get friendly label for search result type
 */
export function getSearchResultTypeLabel(type: string): string {
  switch (type) {
    case 'project':
      return 'Project'
    case 'task':
      return 'Task'
    case 'session':
      return 'Session'
    case 'prompt':
      return 'Prompt'
    case 'code':
      return 'Code'
    default:
      return type.charAt(0).toUpperCase() + type.slice(1)
  }
}
