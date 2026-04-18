import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import {
  ServerKoditAdminRepoListResponse,
  ServerKoditAdminRepoDetailResponse,
  ServerKoditAdminRepositoryTasksResponse,
  ServerKoditAdminStatsResponse,
  ServerKoditAdminBatchResponse,
} from '../api/api'

const POLL_INTERVAL = 5000

// Queue task types (not yet in generated client)
export interface KoditAdminQueueTask {
  id: number
  operation: string
  priority: number
  repository_id: number
  repo_name?: string
  created_at: string
}

export interface KoditAdminQueueStats {
  total: number
  oldest_task_age?: string
  oldest_task_time?: string
  newest_task_time?: string
  by_operation: Record<string, number>
  by_priority_level: Record<string, number>
}

export interface KoditAdminActiveTask {
  operation: string
  state: string
  message?: string
  current: number
  total: number
  repository_id: number
  repo_name?: string
  updated_at: string
}

export interface KoditAdminQueueListResponse {
  active_tasks: KoditAdminActiveTask[]
  data: KoditAdminQueueTask[]
  meta: {
    page: number
    per_page: number
    total: number
    total_pages: number
  }
  stats: KoditAdminQueueStats
}

export const koditAdminReposQueryKey = (page: number, perPage: number) =>
  ['admin', 'kodit', 'repositories', page, perPage]

export const koditAdminRepoDetailQueryKey = (koditRepoId: string) =>
  ['admin', 'kodit', 'repositories', koditRepoId]

export const koditAdminRepoTasksQueryKey = (koditRepoId: string) =>
  ['admin', 'kodit', 'repositories', koditRepoId, 'tasks']

export const koditAdminStatsQueryKey = () => ['admin', 'kodit', 'stats']

export function useAdminKoditStats() {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery<ServerKoditAdminStatsResponse>({
    queryKey: koditAdminStatsQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1AdminKoditStatsList()
      return response.data
    },
    refetchInterval: POLL_INTERVAL,
  })
}

export function useAdminKoditRepositories(page: number, perPage: number) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery<ServerKoditAdminRepoListResponse>({
    queryKey: koditAdminReposQueryKey(page, perPage),
    queryFn: async () => {
      const response = await apiClient.v1AdminKoditRepositoriesList({
        page,
        per_page: perPage,
      })
      return response.data
    },
    refetchInterval: POLL_INTERVAL,
  })
}

export function useAdminKoditRepositoryDetail(koditRepoId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery<ServerKoditAdminRepoDetailResponse>({
    queryKey: koditAdminRepoDetailQueryKey(koditRepoId),
    queryFn: async () => {
      const response = await apiClient.v1AdminKoditRepositoriesDetail(Number(koditRepoId))
      return response.data
    },
    enabled: !!koditRepoId,
    refetchInterval: POLL_INTERVAL,
  })
}

export function useAdminKoditRepositoryTasks(koditRepoId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery<ServerKoditAdminRepositoryTasksResponse>({
    queryKey: koditAdminRepoTasksQueryKey(koditRepoId),
    queryFn: async () => {
      const response = await apiClient.v1AdminKoditRepositoriesTasksDetail(Number(koditRepoId))
      return response.data
    },
    enabled: !!koditRepoId,
    refetchInterval: POLL_INTERVAL,
  })
}

export function useAdminSyncKoditRepository() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (koditRepoId: number) => {
      const response = await apiClient.v1AdminKoditRepositoriesSyncCreate(koditRepoId)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

export function useAdminRescanKoditRepository() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (koditRepoId: number) => {
      const response = await apiClient.v1AdminKoditRepositoriesRescanCreate(koditRepoId)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

export function useAdminDeleteKoditRepository() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (koditRepoId: number) => {
      const response = await apiClient.v1AdminKoditRepositoriesDelete(koditRepoId)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

export function useAdminBatchDeleteKoditRepositories() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation<ServerKoditAdminBatchResponse, Error, number[]>({
    mutationFn: async (ids: number[]) => {
      const response = await apiClient.v1AdminKoditRepositoriesBatchDeleteCreate({ ids })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

export function useAdminBatchRescanKoditRepositories() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation<ServerKoditAdminBatchResponse, Error, number[]>({
    mutationFn: async (ids: number[]) => {
      const response = await apiClient.v1AdminKoditRepositoriesBatchRescanCreate({ ids })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

// =============================================================================
// Admin enrichments & search (by kodit repo ID — works for knowledge repos too)
// =============================================================================

export function useAdminKoditRepoEnrichments(koditRepoId: string, options?: { enabled?: boolean }) {
  const api = useApi()

  return useQuery({
    queryKey: ['kodit', 'repositories', koditRepoId, 'enrichments'],
    queryFn: async () => {
      return api.get(`/api/v1/kodit/repositories/${koditRepoId}/enrichments`)
    },
    enabled: options?.enabled !== false && !!koditRepoId,
  })
}


// =============================================================================
// Queue hooks (using raw API client instance until generated client is updated)
// =============================================================================

export const koditAdminQueueQueryKey = (page: number, perPage: number) =>
  ['admin', 'kodit', 'queue', page, perPage]

export function useAdminKoditQueue(page: number, perPage: number) {
  const api = useApi()

  return useQuery<KoditAdminQueueListResponse>({
    queryKey: koditAdminQueueQueryKey(page, perPage),
    queryFn: async () => {
      const response = await api.get<KoditAdminQueueListResponse>(
        `/api/v1/admin/kodit/queue?page=${page}&per_page=${perPage}`,
      )
      return response as KoditAdminQueueListResponse
    },
    refetchInterval: POLL_INTERVAL,
  })
}

export function useAdminDeleteKoditTask() {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation<void, Error, number>({
    mutationFn: async (taskId: number) => {
      await api.delete(`/api/v1/admin/kodit/queue/${taskId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}

export function useAdminUpdateKoditTaskPriority() {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation<void, Error, { taskId: number; priority: number }>({
    mutationFn: async ({ taskId, priority }) => {
      await api.put(`/api/v1/admin/kodit/queue/${taskId}/priority`, { priority })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'kodit'] })
    },
  })
}
