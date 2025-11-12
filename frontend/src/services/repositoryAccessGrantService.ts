import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { TypesAccessGrant, TypesCreateAccessGrantRequest } from '../api/api'
import useApi from '../hooks/useApi'

// Query key factory
export const repositoryAccessGrantKeys = {
  all: ['repository-access-grants'] as const,
  lists: () => [...repositoryAccessGrantKeys.all, 'list'] as const,
  list: (repositoryId: string) => [...repositoryAccessGrantKeys.lists(), repositoryId] as const,
}

// List repository access grants
export const useListRepositoryAccessGrants = (repositoryId: string, enabled = true) => {
  const api = useApi()

  return useQuery({
    queryKey: repositoryAccessGrantKeys.list(repositoryId),
    queryFn: async () => {
      const response = await api.getApiClient().v1GitRepositoriesAccessGrantsDetail(repositoryId)
      return response.data || []
    },
    enabled: enabled && !!repositoryId,
  })
}

// Create repository access grant
export const useCreateRepositoryAccessGrant = (repositoryId: string) => {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (request: TypesCreateAccessGrantRequest) => {
      const response = await api.getApiClient().v1GitRepositoriesAccessGrantsCreate(repositoryId, request)
      return response.data
    },
    onSuccess: () => {
      // Invalidate and refetch access grants list
      queryClient.invalidateQueries({ queryKey: repositoryAccessGrantKeys.list(repositoryId) })
    },
  })
}

// Delete repository access grant
export const useDeleteRepositoryAccessGrant = (repositoryId: string) => {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (grantId: string) => {
      // Delete endpoint doesn't have Swagger docs, so use api.delete directly
      await api.delete(`/api/v1/git/repositories/${repositoryId}/access-grants/${grantId}`)
    },
    onSuccess: () => {
      // Invalidate and refetch access grants list
      queryClient.invalidateQueries({ queryKey: repositoryAccessGrantKeys.list(repositoryId) })
    },
  })
}
