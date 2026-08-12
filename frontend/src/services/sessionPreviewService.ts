import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import type { TypesVHostRoute } from '../api/api'
import useApi from '../hooks/useApi'

export const SESSION_PREVIEW_TOKENS_QUERY_KEY = (sessionId: string) => [
  'session-preview-tokens',
  sessionId,
]

export function useSessionPreviewTokens(sessionId: string, enabled = true) {
  const api = useApi()

  return useQuery<TypesVHostRoute[]>({
    queryKey: SESSION_PREVIEW_TOKENS_QUERY_KEY(sessionId),
    enabled: enabled && !!sessionId,
    queryFn: async () => {
      const response = await api.getApiClient().v1SessionsPreviewTokensDetail(sessionId)
      return response.data ?? []
    },
  })
}

export function useCreateSessionPreviewToken(sessionId: string) {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (port: number) => {
      const response = await api.getApiClient().v1SessionsPreviewTokensCreate(sessionId, { port })
      return response.data
    },
    onSuccess: () => queryClient.invalidateQueries({
      queryKey: SESSION_PREVIEW_TOKENS_QUERY_KEY(sessionId),
    }),
  })
}

export function useRotateSessionPreviewToken(sessionId: string) {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (tokenId: string) =>
      api.getApiClient().v1SessionsPreviewTokensRotateCreate(sessionId, tokenId),
    onSuccess: () => queryClient.invalidateQueries({
      queryKey: SESSION_PREVIEW_TOKENS_QUERY_KEY(sessionId),
    }),
  })
}

export function useDeleteSessionPreviewToken(sessionId: string) {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (tokenId: string) =>
      api.getApiClient().v1SessionsPreviewTokensDelete(sessionId, tokenId),
    onSuccess: () => queryClient.invalidateQueries({
      queryKey: SESSION_PREVIEW_TOKENS_QUERY_KEY(sessionId),
    }),
  })
}
