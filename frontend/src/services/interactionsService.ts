import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesFeedbackRequest } from '../api/api';

export const appInteractionsQueryKey = (appId: string, sessionId: string, interactionId: string, feedback: string, page: number, pageSize: number) => [
  "app-interactions",
  appId,
  sessionId,
  interactionId,
  feedback,
  page,
  pageSize
];

export function useListAppInteractions(appId: string, session: string, interaction: string, feedback: string, page: number, pageSize: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appInteractionsQueryKey(appId, session, interaction, feedback,page, pageSize),
    queryFn: async () => {
      const response = await apiClient.v1AppsInteractionsDetail(appId, { session, interaction, feedback, page, pageSize })
      return response.data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useUpdateInteractionFeedback(sessionId: string, interactionId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return {
    updateFeedback: async (feedback: TypesFeedbackRequest) => {
      const response = await apiClient.v1SessionsInteractionsFeedbackCreate(sessionId, interactionId, feedback)
      return response.data
    }
  }
}

