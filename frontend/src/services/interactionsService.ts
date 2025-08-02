import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const interactionsQueryKey = (sessionId: string) => [
  "interactions",
  sessionId
];

export const interactionQueryKey = (sessionId: string, interactionId: string) => [
  "interaction",
  sessionId,
  interactionId
];

export const appInteractionsQueryKey = (appId: string, sessionId: string, interactionId: string, page: number, pageSize: number) => [
  "app-interactions",
  appId,
  sessionId,
  interactionId,
  page,
  pageSize
];

export function useListAppInteractions(appId: string, session: string, interaction: string, page: number, pageSize: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appInteractionsQueryKey(appId, session, interaction, page, pageSize),
    queryFn: async () => {
      const response = await apiClient.v1AppsInteractionsDetail(appId, { session, interaction, page, pageSize })
      return response.data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useListInteractions(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: interactionsQueryKey(sessionId),
    queryFn: async () => {
      const response = await apiClient.v1SessionsInteractionsDetail(sessionId)
      return response.data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useGetInteraction(sessionId: string, interactionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: interactionQueryKey(sessionId, interactionId),
    queryFn: async () => {
      const response = await apiClient.v1SessionsInteractionsDetail2(sessionId, interactionId)
      return response.data
    },
    enabled: options?.enabled ?? true,
  })
}

