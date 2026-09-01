import axios from 'axios'
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
export const artifactViewerQueryKey = (artifactId: string) => ['artifact-viewer', artifactId]
export const artifactMarkdownSourceQueryKey = (artifactId: string) => ['artifact-markdown-source', artifactId]

export const artifactMutationData = (form: ArtifactForm) => ({
  name: form.name,
  visibility: form.visibility,
  ...(form.description !== undefined ? { description: form.description } : {}),
  ...(form.entrypoint !== undefined ? { entrypoint: form.entrypoint } : {}),
  ...(form.artifact !== undefined ? { artifact: form.artifact } : {}),
})

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

export const useGetArtifactViewer = (artifactId: string) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: artifactViewerQueryKey(artifactId),
    queryFn: async () => {
      const response = await apiClient.v1PublicArtifactsDetail(artifactId)
      return response.data
    },
    enabled: !!artifactId,
  })
}

export const useGetArtifactMarkdownSource = (artifactId: string, enabled: boolean) => {
  return useQuery<string>({
    queryKey: artifactMarkdownSourceQueryKey(artifactId),
    queryFn: async () => {
      const response = await axios.get<string>(`/artifacts/${artifactId}/document`, {
        responseType: 'text',
        transformResponse: [(data) => data],
      })
      return response.data
    },
    enabled: !!artifactId && enabled,
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
      queryClient.invalidateQueries({ queryKey: artifactViewerQueryKey(artifactId) })
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
      if (!form.artifact) throw new Error('Choose an HTML, Markdown, ZIP, PDF, or image file')
      const response = await apiClient.v1ProjectsArtifactsCreate(projectId, {
        ...artifactMutationData(form),
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
      const response = await apiClient.v1ArtifactsUpdate(id, artifactMutationData(form))
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
