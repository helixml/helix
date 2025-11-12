import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { TypesAccessGrant, TypesCreateAccessGrantRequest } from '../api/api'
import useApi from '../hooks/useApi'

// Query key factory
export const projectAccessGrantKeys = {
  all: ['project-access-grants'] as const,
  lists: () => [...projectAccessGrantKeys.all, 'list'] as const,
  list: (projectId: string) => [...projectAccessGrantKeys.lists(), projectId] as const,
}

// List project access grants
export const useListProjectAccessGrants = (projectId: string, enabled = true) => {
  const api = useApi()

  return useQuery({
    queryKey: projectAccessGrantKeys.list(projectId),
    queryFn: async () => {
      const response = await api.getApiClient().v1ProjectsAccessGrantsDetail(projectId)
      return response.data || []
    },
    enabled: enabled && !!projectId,
  })
}

// Create project access grant
export const useCreateProjectAccessGrant = (projectId: string) => {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (request: TypesCreateAccessGrantRequest) => {
      const response = await api.getApiClient().v1ProjectsAccessGrantsCreate(projectId, request)
      return response.data
    },
    onSuccess: () => {
      // Invalidate and refetch access grants list
      queryClient.invalidateQueries({ queryKey: projectAccessGrantKeys.list(projectId) })
    },
  })
}

// Delete project access grant
export const useDeleteProjectAccessGrant = (projectId: string) => {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (grantId: string) => {
      // Delete endpoint doesn't have Swagger docs, so use api.delete directly
      await api.delete(`/api/v1/projects/${projectId}/access-grants/${grantId}`)
    },
    onSuccess: () => {
      // Invalidate and refetch access grants list
      queryClient.invalidateQueries({ queryKey: projectAccessGrantKeys.list(projectId) })
    },
  })
}
