import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesSession } from '../api/api';
import { QueryClient } from '@tanstack/react-query';

export const SESSION_STEPS_QUERY_KEY = (id: string) => [
  "session-steps",
  id
];

export const GET_SESSION_QUERY_KEY = (id: string) => [
  "session",
  id
];

export const LIST_SESSIONS_QUERY_KEY = (orgId?: string, page?: number, pageSize?: number, search?: string, questionSetExecutionId?: string, projectId ?: string) => [
  "sessions",
  orgId,
  page,
  pageSize,
  search,
  questionSetExecutionId,
  projectId
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListSessionSteps(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: SESSION_STEPS_QUERY_KEY(sessionId),
    queryFn: () => apiClient.v1SessionsStepInfoDetail(sessionId),
    enabled: options?.enabled ?? true
  })
}

export function useGetSession(sessionId: string, options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: GET_SESSION_QUERY_KEY(sessionId),
    queryFn: () => apiClient.v1SessionsDetail(sessionId),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval,
  })
}

export function useListSessions(orgId?: string, search?: string, questionSetExecutionId?: string, projectId?: string, page?: number, pageSize?: number, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  
  return useQuery({
    queryKey: LIST_SESSIONS_QUERY_KEY(orgId, page ?? 0, pageSize ?? 0, search ?? '', questionSetExecutionId ?? '', projectId ?? ''),
    queryFn: () => apiClient.v1SessionsList({
      org_id: orgId,
      search: search,
      question_set_execution_id: questionSetExecutionId,
      page: page,
      page_size: pageSize,
      project_id: projectId,
    }),
    enabled: options?.enabled ?? true
  })
}

export function useUpdateSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: (sessionData: TypesSession) => apiClient.v1SessionsUpdate(sessionId, sessionData),
    onSuccess: (updatedSession) => {
      // Invalidate the specific session query to refresh the session data
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
      // Invalidate all sessions queries to refresh the list after update
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
      // Optionally update the cache directly with the new data
      queryClient.setQueryData(GET_SESSION_QUERY_KEY(sessionId), updatedSession)
    }
  })
}


export function useDeleteSession(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.v1SessionsDelete(sessionId),
    onSuccess: () => {
      // Invalidate all sessions queries to refresh the list after deletion
      queryClient.invalidateQueries({ queryKey: ["sessions"] })
    }
  })
}

export function useStopExternalAgent(sessionId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.v1SessionsStopExternalAgentDelete(sessionId),
    onSuccess: () => {
      // Invalidate session query to refresh wolf-app-state
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
    }
  })
}

export function useGetSessionIdleStatus(sessionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: ['session-idle-status', sessionId],
    queryFn: () => apiClient.v1SessionsIdleStatusDetail(sessionId),
    enabled: options?.enabled ?? true,
    refetchInterval: 30000, // Refetch every 30 seconds to update idle time
  })
}

export function invalidateSessionsQuery(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: ["sessions"] })
}
