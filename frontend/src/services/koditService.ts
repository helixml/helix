import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
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
export const KODIT_SUBTYPE_PHYSICAL = 'physical' // Physical architecture diagrams
export const KODIT_SUBTYPE_API_DOCS = 'api_docs' // API documentation
export const KODIT_SUBTYPE_DATABASE_SCHEMA = 'database_schema' // Database schemas

// Living documentation subtypes
export const KODIT_SUBTYPE_COMMIT_DESCRIPTION = 'commit_description' // Commit descriptions

// Re-export generated types for Kodit indexing status
export type { ServerKoditIndexingStatusDTO as KoditIndexingStatus } from '../api/api'

/**
 * Query key factory for Kodit enrichments
 */
export const koditEnrichmentsQueryKey = (repoId: string, commitSha?: string) =>
  commitSha ? ['kodit', 'enrichments', repoId, commitSha] : ['kodit', 'enrichments', repoId]
export const koditEnrichmentDetailQueryKey = (repoId: string, enrichmentId: string) =>
  ['kodit', 'enrichments', repoId, enrichmentId]
export const koditCommitsQueryKey = (repoId: string) => ['kodit', 'commits', repoId]
export const koditStatusQueryKey = (repoId: string) => ['kodit', 'status', repoId]
export const koditSearchQueryKey = (repoId: string, query: string) => ['kodit', 'search', repoId, query]
export const koditWikiTreeQueryKey = (repoId: string) => ['kodit', 'wiki', 'tree', repoId]
export const koditWikiPageQueryKey = (repoId: string, pagePath: string) => ['kodit', 'wiki', 'page', repoId, pagePath]
export const koditSemanticSearchQueryKey = (repoId: string, query: string) => ['kodit', 'semantic-search', repoId, query]
export const koditKeywordSearchQueryKey = (repoId: string, keywords: string) => ['kodit', 'keyword-search', repoId, keywords]
export const koditGrepQueryKey = (repoId: string, pattern: string) => ['kodit', 'grep', repoId, pattern]
export const koditFilesQueryKey = (repoId: string, pattern: string) => ['kodit', 'files', repoId, pattern]
export const koditFileContentQueryKey = (repoId: string, path: string) => ['kodit', 'file-content', repoId, path]

/**
 * Hook to fetch code intelligence enrichments for a repository
 *
 * @param repoId - Repository ID
 * @param commitSha - Optional commit SHA to filter enrichments
 * @param options - Query options
 * @param options.enabled - Whether to enable the query
 * @param options.refetchInterval - Custom refetch interval in ms (default: 30s for latest, undefined for specific commits)
 *                                  Set to a lower value (e.g., 3000) during active indexing to see enrichments flow in
 */
export function useKoditEnrichments(repoId: string, commitSha?: string, options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  // Determine refetch interval:
  // - If explicitly provided, use that value
  // - For specific commits: no auto-refetch (data won't change)
  // - For latest: refetch every 30 seconds by default
  const defaultRefetchInterval = commitSha ? undefined : 30 * 1000
  const refetchInterval = options?.refetchInterval !== undefined ? options.refetchInterval : defaultRefetchInterval

  return useQuery({
    queryKey: koditEnrichmentsQueryKey(repoId, commitSha),
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesEnrichmentsDetail(repoId, {
        commit_sha: commitSha,
      })
      return response.data
    },
    enabled: options?.enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval,
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
 * Hook to search code snippets in a repository
 */
export function useKoditSearch(
  repoId: string,
  query: string,
  limit?: number,
  commitSha?: string,
  options?: { enabled?: boolean }
) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: [...koditSearchQueryKey(repoId, query), commitSha],
    queryFn: async () => {
      const response = await apiClient.v1GitRepositoriesSearchSnippetsDetail(repoId, {
        query,
        limit,
      })
      // Response is an array of snippets
      const data = response.data as any
      return Array.isArray(data) ? data : []
    },
    enabled: options?.enabled !== false && !!repoId && !!query && query.trim().length > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to trigger a rescan of a specific commit in Kodit
 * This will refresh the code intelligence indexing for that commit
 */
export function useKoditRescan(repoId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (commitSha: string) => {
      const response = await apiClient.v1GitRepositoriesKoditRescanCreate(repoId, { commit_sha: commitSha })
      return response.data
    },
    onSuccess: () => {
      // Remove all kodit data completely to force fresh fetches
      // This ensures the UI shows loading states and fetches fresh data
      queryClient.removeQueries({ queryKey: ['kodit', 'enrichments', repoId] })
      queryClient.removeQueries({ queryKey: ['kodit', 'search', repoId] })
      queryClient.removeQueries({ queryKey: ['kodit', 'status', repoId] })
      queryClient.removeQueries({ queryKey: ['kodit', 'commits', repoId] })
    },
  })
}

/**
 * Wiki tree node type
 */
export interface KoditWikiTreeNode {
  slug: string
  title: string
  path: string
  links?: Record<string, string>
  children?: KoditWikiTreeNode[]
}

/**
 * Wiki page type
 */
export interface KoditWikiPage {
  slug: string
  title: string
  content: string
}

/**
 * Hook to fetch the wiki navigation tree for a repository
 */
export function useKoditWikiTree(repoId: string, options?: { enabled?: boolean }) {
  const api = useApi()

  return useQuery({
    queryKey: koditWikiTreeQueryKey(repoId),
    queryFn: async () => {
      const response = await api.get<{ data: KoditWikiTreeNode[], links: Record<string, string> }>(`/api/v1/git/repositories/${repoId}/wiki`)
      return response?.data || []
    },
    enabled: options?.enabled !== false && !!repoId,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch a single wiki page by path
 */
export function useKoditWikiPage(repoId: string, pagePath: string, options?: { enabled?: boolean }) {
  const api = useApi()

  return useQuery({
    queryKey: koditWikiPageQueryKey(repoId, pagePath),
    queryFn: async () => {
      const response = await api.get<{ data: KoditWikiPage, links: Record<string, string> }>(`/api/v1/git/repositories/${repoId}/wiki-page?path=${encodeURIComponent(pagePath)}`)
      return response?.data
    },
    enabled: options?.enabled !== false && !!repoId && !!pagePath,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Search result types
 */
export interface KoditFileResult {
  path: string
  language: string
  lines: string
  score: number
  preview: string
  links?: Record<string, string>
}

export interface KoditGrepMatch {
  line: number
  content: string
}

export interface KoditGrepResult {
  path: string
  language: string
  matches: KoditGrepMatch[]
  links?: Record<string, string>
}

export interface KoditFileEntry {
  path: string
  size: number
  links?: Record<string, string>
}

export interface KoditFileContent {
  path: string
  content: string
  commit_sha: string
}

/**
 * Generic envelope types for JSON:API-style responses
 */
export interface KoditSearchMeta {
  query: string
  limit: number
  language?: string
  count: number
}

interface KoditSearchResponse {
  data: KoditFileResult[]
  meta: KoditSearchMeta
  links: Record<string, string>
}

export interface KoditGrepMeta {
  pattern: string
  glob?: string
  limit: number
  count: number
}

interface KoditGrepResponse {
  data: KoditGrepResult[]
  meta: KoditGrepMeta
  links: Record<string, string>
}

export interface KoditFilesMeta {
  pattern: string
  count: number
}

interface KoditFilesResponse {
  data: KoditFileEntry[]
  meta: KoditFilesMeta
  links: Record<string, string>
}

interface KoditFileContentResponse {
  data: KoditFileContent
  links: Record<string, string>
}

/**
 * Hook for semantic (vector similarity) search
 */
export function useKoditSemanticSearch(
  repoId: string,
  query: string,
  limit?: number,
  language?: string,
  options?: { enabled?: boolean }
) {
  const api = useApi()

  return useQuery({
    queryKey: [...koditSemanticSearchQueryKey(repoId, query), language],
    queryFn: async () => {
      const params = new URLSearchParams({ query })
      if (limit) params.set('limit', String(limit))
      if (language) params.set('language', language)
      const response = await api.get<KoditSearchResponse>(`/api/v1/git/repositories/${repoId}/semantic-search?${params}`)
      return { data: response?.data || [], meta: response?.meta }
    },
    enabled: options?.enabled !== false && !!repoId && !!query && query.trim().length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook for keyword (BM25) search
 */
export function useKoditKeywordSearch(
  repoId: string,
  keywords: string,
  limit?: number,
  language?: string,
  options?: { enabled?: boolean }
) {
  const api = useApi()

  return useQuery({
    queryKey: [...koditKeywordSearchQueryKey(repoId, keywords), language],
    queryFn: async () => {
      const params = new URLSearchParams({ keywords })
      if (limit) params.set('limit', String(limit))
      if (language) params.set('language', language)
      const response = await api.get<KoditSearchResponse>(`/api/v1/git/repositories/${repoId}/keyword-search?${params}`)
      return { data: response?.data || [], meta: response?.meta }
    },
    enabled: options?.enabled !== false && !!repoId && !!keywords && keywords.trim().length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook for grep (regex pattern search via git grep)
 */
export function useKoditGrep(
  repoId: string,
  pattern: string,
  glob?: string,
  limit?: number,
  options?: { enabled?: boolean }
) {
  const api = useApi()

  return useQuery({
    queryKey: [...koditGrepQueryKey(repoId, pattern), glob],
    queryFn: async () => {
      const params = new URLSearchParams({ pattern })
      if (glob) params.set('glob', glob)
      if (limit) params.set('limit', String(limit))
      const response = await api.get<KoditGrepResponse>(`/api/v1/git/repositories/${repoId}/grep?${params}`)
      return { data: response?.data || [], meta: response?.meta }
    },
    enabled: options?.enabled !== false && !!repoId && !!pattern && pattern.trim().length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook for listing files (glob pattern matching)
 */
export function useKoditFiles(
  repoId: string,
  pattern: string,
  options?: { enabled?: boolean }
) {
  const api = useApi()

  return useQuery({
    queryKey: koditFilesQueryKey(repoId, pattern),
    queryFn: async () => {
      const params = new URLSearchParams()
      if (pattern) params.set('pattern', pattern)
      const response = await api.get<KoditFilesResponse>(`/api/v1/git/repositories/${repoId}/files?${params}`)
      return { data: response?.data || [], meta: response?.meta }
    },
    enabled: options?.enabled !== false && !!repoId && !!pattern && pattern.trim().length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook for reading file content
 */
export function useKoditFileContent(
  repoId: string,
  filePath: string,
  options?: { enabled?: boolean; startLine?: number; endLine?: number }
) {
  const api = useApi()

  return useQuery({
    queryKey: koditFileContentQueryKey(repoId, filePath),
    queryFn: async () => {
      const params = new URLSearchParams({ path: filePath })
      if (options?.startLine) params.set('start_line', String(options.startLine))
      if (options?.endLine) params.set('end_line', String(options.endLine))
      const response = await api.get<KoditFileContentResponse>(`/api/v1/git/repositories/${repoId}/file-content?${params}`)
      return response?.data
    },
    enabled: options?.enabled !== false && !!repoId && !!filePath,
    staleTime: 5 * 60 * 1000,
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
    [KODIT_SUBTYPE_PHYSICAL]: 'Physical Architecture',
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
    [KODIT_SUBTYPE_PHYSICAL]: '🏛️',
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

// =============================================================================
// Hooks for kodit repo ID-based endpoints (works for both git and knowledge repos)
// =============================================================================

export function useKoditRepoEnrichments(koditRepoId: number | undefined, page = 1, perPage = 25, options?: { enabled?: boolean }) {
  const api = useApi()

  return useQuery({
    queryKey: ['kodit', 'repositories', koditRepoId, 'enrichments', page, perPage],
    queryFn: async () => {
      return api.get(`/api/v1/kodit/repositories/${koditRepoId}/enrichments?page=${page}&per_page=${perPage}`)
    },
    enabled: options?.enabled !== false && !!koditRepoId,
  })
}
