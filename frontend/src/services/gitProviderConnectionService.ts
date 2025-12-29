import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import type { TypesGitProviderConnection, TypesGitProviderConnectionCreateRequest } from '../api/api'

// Query keys
const CONNECTIONS_KEY = ['git-provider-connections']
const connectionRepositoriesKey = (id: string) => ['git-provider-connections', id, 'repositories']

// List all git provider connections for the current user
export function useGitProviderConnections() {
  const api = useApi()

  return useQuery({
    queryKey: CONNECTIONS_KEY,
    queryFn: async () => {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitProviderConnectionsList()
      return response.data
    },
  })
}

// Create a new git provider connection
export function useCreateGitProviderConnection() {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (request: TypesGitProviderConnectionCreateRequest) => {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitProviderConnectionsCreate(request)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CONNECTIONS_KEY })
    },
  })
}

// Delete a git provider connection
export function useDeleteGitProviderConnection() {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (id: string) => {
      const apiClient = api.getApiClient()
      await apiClient.v1GitProviderConnectionsDelete(id)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CONNECTIONS_KEY })
    },
  })
}

// Browse repositories for a saved connection
export function useGitProviderConnectionRepositories(connectionId: string) {
  const api = useApi()

  return useQuery({
    queryKey: connectionRepositoriesKey(connectionId),
    queryFn: async () => {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitProviderConnectionsRepositoriesDetail(connectionId)
      return response.data
    },
    enabled: !!connectionId,
  })
}
