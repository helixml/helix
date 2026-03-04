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
