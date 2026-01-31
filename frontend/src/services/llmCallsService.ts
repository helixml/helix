import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const llmCallsQueryKey = (session: string, interaction: string) => [
  "llm_calls",
  session,
  interaction
];

export const appLLMCallsQueryKey = (appId: string, session: string, interaction: string) => [
  "app_llm_calls",
  appId,
  session,
  interaction
];

export function useListLLMCalls(session: string, interaction: string, page: number, pageSize: number, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: llmCallsQueryKey(session, interaction),
    queryFn: async () => {
      const response = await apiClient.v1LlmCallsList({
        session,
        interaction,
        page: page,
        pageSize: pageSize,
      })
      return response.data
    },    
    enabled: enabled,
  })
}

export function useListAppLLMCalls(appId: string, session: string, interaction: string, page: number, pageSize: number, enabled?: boolean, refetchInterval?: number) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appLLMCallsQueryKey(appId, session, interaction),
    queryFn: async () => {
      const response = await apiClient.v1AppsLlmCallsDetail(appId, {
        session,
        interaction,
        page: page,
        pageSize: pageSize,
      })
      return response.data
    },    
    enabled: enabled,
    refetchInterval: refetchInterval ? refetchInterval : undefined,
  })
}