import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

// Page/pageSize must be part of the key — the query fn closes over them, so
// with a page-less key a page change would keep serving the cached first page.
export const llmCallsQueryKey = (session: string, interaction: string, page: number, pageSize: number) => [
  "llm_calls",
  session,
  interaction,
  page,
  pageSize
];

export const appLLMCallsQueryKey = (appId: string, session: string, interaction: string, page: number, pageSize: number) => [
  "app_llm_calls",
  appId,
  session,
  interaction,
  page,
  pageSize
];

export function useListLLMCalls(session: string, interaction: string, page: number, pageSize: number, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: llmCallsQueryKey(session, interaction, page, pageSize),
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
    queryKey: appLLMCallsQueryKey(appId, session, interaction, page, pageSize),
    queryFn: async () => {
      const response = await apiClient.v1AgentsLlmCallsDetail(appId, {
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