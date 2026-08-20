import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { TypesArtifact } from '../api/api'
import useApi from '../hooks/useApi'

export type ArtifactForm = {
  name: string
  description?: string
  entrypoint?: string
  visibility: 'project' | 'public'
  artifact?: File
}

export const projectArtifactsQueryKey = (projectId: string) => ['project-artifacts', projectId]
export const artifactQueryKey = (artifactId: string) => ['artifact', artifactId]

export const useGetArtifact = (artifactId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery<TypesArtifact>({
    queryKey: artifactQueryKey(artifactId),
    queryFn: async () => {
      const response = await apiClient.v1ArtifactsDetail(artifactId)
      return response.data
    },
    enabled: !!artifactId,
  })
}

export const useSetArtifactVisibility = (artifactId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (visibility: 'project' | 'public') => {
      const response = await apiClient.v1ArtifactsUpdate(artifactId, { visibility })
      return response.data
    },
    onSuccess: (artifact) => {
      queryClient.invalidateQueries({ queryKey: artifactQueryKey(artifactId) })
      if (artifact.project_id) {
        queryClient.invalidateQueries({ queryKey: projectArtifactsQueryKey(artifact.project_id) })
      }
    },
  })
}

export const useListProjectArtifacts = (projectId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery<TypesArtifact[]>({
    queryKey: projectArtifactsQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsArtifactsDetail(projectId)
      return response.data.artifacts ?? []
    },
    enabled: !!projectId,
  })
}

export const useCreateArtifact = (projectId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: ArtifactForm) => {
      if (!form.artifact) throw new Error('Choose an HTML file or ZIP bundle')
      const response = await apiClient.v1ProjectsArtifactsCreate(projectId, {
        ...form,
        artifact: form.artifact,
      })
      return response.data
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectArtifactsQueryKey(projectId) }),
  })
}

export const useUpdateArtifact = (projectId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, form }: { id: string; form: ArtifactForm }) => {
      const response = await apiClient.v1ArtifactsUpdate(id, form)
      return response.data
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectArtifactsQueryKey(projectId) }),
  })
}

export const useDeleteArtifact = (projectId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (artifactId: string) => apiClient.v1ArtifactsDelete(artifactId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectArtifactsQueryKey(projectId) }),
  })
}
